package terminal

import (
	"strings"
	"sync"

	"github.com/hinshun/vt10x"
)

// ScrollbackCell 是回滚缓冲区中单个字符的完整属性快照。
// 保留 vt10x.Glyph 的原始 Color/Mode 值而非已解析的 color.Color，
// 以保留 DefaultFG/DefaultBG 语义（主题切换时仍能正确渲染），
// 并由 render.go 的 glyphColor 在渲染时解析为具体颜色。
type ScrollbackCell struct {
	Char rune
	FG   vt10x.Color
	BG   vt10x.Color
	Mode int16 // 与 vt10x.Glyph.Mode 一致：attrReverse/attrBold/attrItalic/attrUnderline 等
}

// ScrollbackLine 是回滚缓冲区中的一行（属性化 cell 序列）。
type ScrollbackLine struct {
	Cells []ScrollbackCell
}

// String 返回该行的纯文本（用于搜索等不需要属性的场景）。
// 尾部不裁剪空格，调用方按需处理。
func (l ScrollbackLine) String() string {
	if len(l.Cells) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.Grow(len(l.Cells))
	for _, c := range l.Cells {
		ch := c.Char
		if ch == 0 {
			ch = ' '
		}
		sb.WriteRune(ch)
	}
	return sb.String()
}

// Scrollback 是终端回滚缓冲区。
// 当屏幕内容滚动出可见区域顶部时（正常输出滚动、resize、清屏），
// 被推出的行通过 PushLine 压入缓冲区，供用户向上滚动查看历史输出。
type Scrollback struct {
	mu       sync.Mutex
	lines    []ScrollbackLine
	maxLines int
	head     int // 环形缓冲区的写入位置
	count    int // 已压入的总行数（不含被覆盖的）
}

// NewScrollback 创建一个最大容量为 maxLines 的回滚缓冲区。
// 若 maxLines <= 0 则使用默认值 10000。
func NewScrollback(maxLines int) *Scrollback {
	if maxLines <= 0 {
		maxLines = 10000
	}
	return &Scrollback{maxLines: maxLines}
}

// PushLine 向缓冲区追加一行属性化文本。
// 当缓冲区已满时采用环形覆盖策略，覆盖最旧的行。
func (s *Scrollback) PushLine(line ScrollbackLine) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.lines) < s.maxLines {
		s.lines = append(s.lines, line)
		s.count++
		return
	}
	s.lines[s.head] = line
	s.head = (s.head + 1) % s.maxLines
}

// Lines 返回缓冲区中所有行的快照（按时间顺序，旧 -> 新）。
func (s *Scrollback) Lines() []ScrollbackLine {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.lines) < s.maxLines {
		out := make([]ScrollbackLine, len(s.lines))
		copy(out, s.lines)
		return out
	}
	out := make([]ScrollbackLine, s.maxLines)
	for i := 0; i < s.maxLines; i++ {
		out[i] = s.lines[(s.head+i)%s.maxLines]
	}
	return out
}

// Clear 清空缓冲区。
func (s *Scrollback) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lines = s.lines[:0]
	s.head = 0
	s.count = 0
}

// Count 返回已压入的行数（不超过 maxLines）。
func (s *Scrollback) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.count
}

// CaptureScreenCells 从 vt10x.Terminal 读取当前可见屏幕的属性化行。
// 每个 cell 包含字符、前景色、背景色和样式模式位。
// 调用方无需持有 term 的锁，本函数会自行加锁。
func CaptureScreenCells(term vt10x.Terminal, cols, rows int) []ScrollbackLine {
	term.Lock()
	defer term.Unlock()
	w, h := term.Size()
	if w != cols {
		cols = w
	}
	if h != rows {
		rows = h
	}
	if rows <= 0 || cols <= 0 {
		return nil
	}
	lines := make([]ScrollbackLine, rows)
	for y := 0; y < rows; y++ {
		cells := make([]ScrollbackCell, cols)
		for x := 0; x < cols; x++ {
			g := term.Cell(x, y)
			cells[x] = ScrollbackCell{
				Char: g.Char,
				FG:   g.FG,
				BG:   g.BG,
				Mode: g.Mode,
			}
		}
		lines[y] = ScrollbackLine{Cells: cells}
	}
	return lines
}

// CaptureScreen 从 vt10x.Terminal 读取当前可见屏幕的纯文本行。
// 每行会去除尾部多余空格，空行返回空字符串。
// 仅供搜索等不需要属性的场景使用；需要属性化数据请用 CaptureScreenCells。
// 调用方无需持有 term 的锁，本函数会自行加锁。
func CaptureScreen(term vt10x.Terminal, cols, rows int) []string {
	term.Lock()
	defer term.Unlock()
	w, h := term.Size()
	if w != cols {
		cols = w
	}
	if h != rows {
		rows = h
	}
	if rows <= 0 {
		return nil
	}
	lines := make([]string, rows)
	for y := 0; y < rows; y++ {
		var sb strings.Builder
		lastNonSpace := -1
		for x := 0; x < cols; x++ {
			g := term.Cell(x, y)
			ch := g.Char
			if ch == 0 {
				ch = ' '
			}
			sb.WriteRune(ch)
			if ch != ' ' {
				lastNonSpace = x
			}
		}
		line := sb.String()
		if lastNonSpace >= 0 && lastNonSpace < len(line)-1 {
			line = line[:lastNonSpace+1]
		} else if lastNonSpace < 0 {
			line = ""
		}
		lines[y] = line
	}
	return lines
}
