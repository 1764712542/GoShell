package terminal

import (
	"os/exec"
	"runtime"
	"sync"
)

// Hyperlink 描述一个 OSC 8 超链接区域。
// 终端输出中的 OSC 8 序列 (ESC ] 8 ; params ; URL ST text ESC ] 8 ;; ST)
// 会创建一个 Hyperlink，记录 URL 和覆盖的单元格范围。
type Hyperlink struct {
	URL            string
	StartX, StartY int // 起始单元格坐标 (0-based)
	EndX, EndY     int // 结束单元格坐标 (0-based, inclusive)
}

// HyperlinkManager 管理终端输出中的 OSC 8 超链接。
//
// 由于 vt10x 库本身不解析 OSC 8 序列，HyperlinkManager 在数据写入
// Emulator 之前扫描数据流，提取超链接信息并记录每个单元格对应的 URL。
// 渲染层可通过 URLAt(x, y) 查询某个单元格是否属于超链接。
type HyperlinkManager struct {
	mu         sync.RWMutex
	hyperlinks []Hyperlink       // 所有活跃的超链接
	cellURLs   map[uint32]string // 单元格 -> URL 的快速索引 (key = y*cols + x)

	// 解析状态机
	parser parserState
	// 当前正在收集的 URL
	currentURL string
	// 当前超链接的起始位置（由外部设置）
	currentStartX int
	currentStartY int
}

// parserState 表示 OSC 8 解析器的状态。
type parserState int

const (
	parserNormal    parserState = iota
	parserEsc                   // 收到 ESC
	parserOSCStart              // 收到 ESC ]，等待 OSC 编号
	parserOSCParams             // 在 OSC 8 的参数部分
	parserOSCURL                // 在 OSC 8 的 URL 部分
	parserOSCEnd                // 等待 ST (ESC \ 或 BEL)
)

// cellKey 生成单元格索引的唯一键。
func cellKey(x, y int) uint32 {
	return uint32(y)<<16 | uint32(x&0xffff)
}

// NewHyperlinkManager 创建一个超链接管理器。
func NewHyperlinkManager() *HyperlinkManager {
	return &HyperlinkManager{
		cellURLs: make(map[uint32]string),
	}
}

// Scan 扫描数据流，提取 OSC 8 超链接信息。
// 应在数据写入 Emulator 之前调用。
// cursorX/cursorY 为当前光标位置（用于记录超链接起始坐标）。
// cols 为终端列数，用于计算换行后的位置。
// 返回处理后的数据（OSC 8 序列会被移除，仅保留可见文本）。
func (h *HyperlinkManager) Scan(data []byte, cursorX, cursorY, cols int) []byte {
	h.mu.Lock()
	defer h.mu.Unlock()

	var result []byte
	x, y := cursorX, cursorY

	for i := 0; i < len(data); i++ {
		c := data[i]

		switch h.parser {
		case parserNormal:
			if c == 0x1b {
				h.parser = parserEsc
				continue
			}
			// 普通字符：如果当前在超链接区域内，记录 URL
			if h.currentURL != "" {
				h.cellURLs[cellKey(x, y)] = h.currentURL
			}
			result = append(result, c)
			x++
			if x >= cols {
				x = 0
				y++
			}

		case parserEsc:
			if c == ']' {
				h.parser = parserOSCStart
				// 保存当前位置作为可能的超链接起始
				h.currentStartX = x
				h.currentStartY = y
				continue
			}
			// 不是 OSC，将 ESC 和当前字符放回结果
			result = append(result, 0x1b, c)
			h.parser = parserNormal

		case parserOSCStart:
			// 期望 '8'，后跟 ';' 或其他 OSC 编号
			if c == '8' {
				h.parser = parserOSCParams
				continue
			}
			// 不是 OSC 8，将整个序列放回结果
			result = append(result, 0x1b, ']', c)
			h.parser = parserNormal

		case parserOSCParams:
			// 跳过参数直到 ';' 分隔 URL
			if c == ';' {
				h.parser = parserOSCURL
				h.currentURL = ""
				continue
			}
			// 如果遇到 ST 或 BEL，结束 OSC（无 URL）
			if c == 0x07 {
				h.parser = parserNormal
				continue
			}
			if c == 0x1b {
				h.parser = parserOSCEnd
				continue
			}

		case parserOSCURL:
			// 收集 URL 直到 ST (ESC \) 或 BEL
			if c == 0x07 {
				// BEL 结束 OSC
				h.beginHyperlink(x, y)
				h.parser = parserNormal
				continue
			}
			if c == 0x1b {
				h.parser = parserOSCEnd
				continue
			}
			h.currentURL += string(c)

		case parserOSCEnd:
			// 等待 '\\' (ST 的第二个字节)
			if c == '\\' {
				// 如果当前在收集 URL，开始一个超链接
				if h.currentURL != "" || h.parser == parserOSCURL {
					// 检查是否是结束序列 (空 URL)
					if h.currentURL == "" {
						// 结束当前超链接
						h.endHyperlink(x, y)
					} else {
						h.beginHyperlink(x, y)
					}
				}
				h.parser = parserNormal
				continue
			}
			// 不是 ST，可能是 ESC 后跟其他字符
			if h.parser == parserOSCURL {
				h.currentURL += string(rune(0x1b)) + string(rune(c))
				h.parser = parserOSCURL
			} else {
				h.parser = parserNormal
			}
		}
	}

	return result
}

// beginHyperlink 开始一个新的超链接区域。
func (h *HyperlinkManager) beginHyperlink(x, y int) {
	// 在第一个可见字符之前调用，记录起始位置
	h.currentStartX = x
	h.currentStartY = y
}

// endHyperlink 结束当前超链接区域并保存。
func (h *HyperlinkManager) endHyperlink(endX, endY int) {
	if h.currentURL == "" {
		return
	}
	link := Hyperlink{
		URL:    h.currentURL,
		StartX: h.currentStartX,
		StartY: h.currentStartY,
		EndX:   endX,
		EndY:   endY,
	}
	h.hyperlinks = append(h.hyperlinks, link)
	h.currentURL = ""
}

// URLAt 返回指定单元格位置的 URL（若属于某个超链接）。
// 否则返回空字符串。
func (h *HyperlinkManager) URLAt(x, y int) string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.cellURLs[cellKey(x, y)]
}

// Hyperlinks 返回所有当前已解析的超链接列表（快照）。
func (h *HyperlinkManager) Hyperlinks() []Hyperlink {
	h.mu.RLock()
	defer h.mu.RUnlock()
	result := make([]Hyperlink, len(h.hyperlinks))
	copy(result, h.hyperlinks)
	return result
}

// Clear 清除所有超链接记录。
func (h *HyperlinkManager) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.hyperlinks = nil
	h.cellURLs = make(map[uint32]string)
	h.currentURL = ""
	h.parser = parserNormal
}

// Reset 重置解析器状态（不清除已记录的超链接）。
// 在终端清屏或切换 alt screen 时调用。
func (h *HyperlinkManager) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.parser = parserNormal
	h.currentURL = ""
}

// OpenURL 使用系统默认浏览器打开 URL。
// 跨平台支持：macOS 用 open，Linux 用 xdg-open，Windows 用 rundll32。
func OpenURL(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return exec.Command("open", url).Start()
	}
}
