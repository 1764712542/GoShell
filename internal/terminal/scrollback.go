package terminal

import (
	"strings"
	"sync"

	"github.com/hinshun/vt10x"
)

// Scrollback 是终端回滚缓冲区。
// 当屏幕内容被滚动出可见区域时（例如 resize 或清屏），
// 通过 CaptureScreenToScrollback 捕获当前屏幕行并压入缓冲区，
// 供用户向上滚动查看历史输出。
type Scrollback struct {
	mu       sync.Mutex
	lines    []string
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

// PushLine 向缓冲区追加一行文本。
// 当缓冲区已满时采用环形覆盖策略，覆盖最旧的行。
func (s *Scrollback) PushLine(line string) {
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
func (s *Scrollback) Lines() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.lines) < s.maxLines {
		out := make([]string, len(s.lines))
		copy(out, s.lines)
		return out
	}
	out := make([]string, s.maxLines)
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

// CaptureScreen 从 vt10x.Terminal 读取当前可见屏幕的文本行。
// 每行会去除尾部多余空格，空行返回空字符串。
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
