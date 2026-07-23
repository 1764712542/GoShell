// Package terminal 提供终端模拟器的核心能力，包括 VT100 解析、
// Fyne 渲染、键盘事件转换以及 ZMODEM 文件接收。
package terminal

import (
	"sync"

	"github.com/hinshun/vt10x"
)

// Emulator 对 vt10x.Terminal 进行线程安全封装。
//
// vt10x 内部自带 sync.Mutex（State.mu），Write/Resize 会自动加锁，
// Lock/Unlock 暴露给外部用于批量读取时的同步。Emulator 额外维护一个
// 独立的 mu 仅用于保护 changed 标志，避免与 vt10x 的锁产生耦合。
//
// 线程安全约定：
//   - Write/Resize 可在任意 goroutine 调用（vt10x 内部加锁）
//   - Cell/Cursor/Mode 等读取方法必须在 Lock/Unlock 区间内调用
//   - Size 仅读取 int 值，可在任意 goroutine 调用
type Emulator struct {
	term vt10x.Terminal

	mu      sync.Mutex // 仅保护 changed 和 scrollback
	changed bool       // 是否有更新需要重绘

	// bracketedPaste 跟踪 DECSET 2004（Bracketed Paste Mode）。
	// vt10x 库本身不解析 mode 2004，因此由 Emulator 自行维护。
	bracketedPaste bool

	// scrollback 是回滚缓冲区引用，由外部（TerminalWidget）通过
	// SetScrollback 注入。vt10x 本身不支持 scrollback——当内容滚动出
	// 屏幕顶部时直接丢弃。Emulator 在 Write 中通过写入前后对比检测
	// 滚动，把被推出屏幕顶部的行压入 scrollback。
	scrollback *Scrollback
}

// NewEmulator 创建一个指定尺寸的终端模拟器。
func NewEmulator(cols, rows int) *Emulator {
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	return &Emulator{
		term:    vt10x.New(vt10x.WithSize(cols, rows)),
		changed: true,
	}
}

// Write 线程安全地写入数据，触发 ANSI 解析并更新内部网格。
// vt10x 的 Write 内部会加锁，写完后标记 changed=true 通知 UI 重绘。
// 同时扫描数据流中的 DECSET/DECRST 2004 序列以跟踪 Bracketed Paste Mode。
//
// 滚动检测：vt10x 不维护 scrollback，当光标在屏幕底部遇到换行时，
// scrollUp 会把屏幕顶部的行直接丢弃。Emulator 在写入前对屏幕做属性化
// 快照，写入后通过比较前后状态推断有多少行滚出顶部，并把这些行压入
// scrollback。在 alt screen 模式下（vim/htop 等全屏程序）不捕获，
// 以免污染主缓冲区历史。
func (e *Emulator) Write(data []byte) (int, error) {
	// 在写入前扫描 mode 2004 控制序列
	e.scanBracketedPasteMode(data)

	// 写入前对主屏幕做属性化快照（仅在主屏模式且有 scrollback 时）
	var snapshot []ScrollbackLine
	if e.shouldCaptureScrollback() {
		snapshot = e.captureScreenForScroll()
	}

	n, err := e.term.Write(data)

	if n > 0 && snapshot != nil {
		e.detectAndPushScrollback(snapshot)
	}

	if n > 0 {
		e.mu.Lock()
		e.changed = true
		e.mu.Unlock()
	}
	return n, err
}

// SetScrollback 注入回滚缓冲区。由 TerminalWidget 在构造时调用。
// 传入 nil 可禁用滚动捕获。
func (e *Emulator) SetScrollback(sb *Scrollback) {
	e.mu.Lock()
	e.scrollback = sb
	e.mu.Unlock()
}

// shouldCaptureScrollback 判断当前是否需要做滚动捕获。
// 条件：已注入 scrollback，且不在 alt screen 模式。
func (e *Emulator) shouldCaptureScrollback() bool {
	e.mu.Lock()
	sb := e.scrollback
	e.mu.Unlock()
	if sb == nil {
		return false
	}
	e.term.Lock()
	alt := e.term.Mode()&vt10x.ModeAltScreen != 0
	e.term.Unlock()
	return !alt
}

// captureScreenForScroll 在写入前对当前主屏幕做完整属性化快照。
// 调用方需确保此时不在 alt screen 模式（由 shouldCaptureScrollback 保证）。
func (e *Emulator) captureScreenForScroll() []ScrollbackLine {
	e.term.Lock()
	defer e.term.Unlock()
	cols, rows := e.term.Size()
	if rows <= 0 || cols <= 0 {
		return nil
	}
	lines := make([]ScrollbackLine, rows)
	for y := 0; y < rows; y++ {
		cells := make([]ScrollbackCell, cols)
		for x := 0; x < cols; x++ {
			g := e.term.Cell(x, y)
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

// detectAndPushScrollback 通过比较写入前后的屏幕内容，推断有多少行
// 从顶部滚出，并把它们压入 scrollback。
//
// 算法：若发生了 N 行滚动，则写入前的 prev[N..rows-1] 应与写入后的
// new[0..rows-1-N] 逐 cell 对齐。因此查找最小的 N>=1 使得 prev[N] 与
// new[0] 完全一致，N 即为滚出行数。若 prev[0]==new[0] 视为未滚动；
// 若找不到对齐位置（行 0 被原地改写而非滚动）也视为未滚动。
//
// 局限：单次 Write 内同时改写顶部行又触发滚动的极端场景下，可能漏推
// 被改写后又滚出的行——这是 before/after 对比法的固有局限，对常规
// 终端输出（追加式输出、逐行滚动）完全正确。
func (e *Emulator) detectAndPushScrollback(prev []ScrollbackLine) {
	if len(prev) == 0 {
		return
	}
	e.term.Lock()
	defer e.term.Unlock()
	cols, rows := e.term.Size()
	if rows <= 0 || cols <= 0 {
		return
	}

	// rowEqualsCurrent 判断 prev[y] 是否与当前屏幕第 y 行完全一致。
	rowEqualsCurrent := func(pl ScrollbackLine, y int) bool {
		cells := pl.Cells
		if len(cells) < cols {
			return false
		}
		for x := 0; x < cols; x++ {
			g := e.term.Cell(x, y)
			if g.Char != cells[x].Char || g.Mode != cells[x].Mode ||
				g.FG != cells[x].FG || g.BG != cells[x].BG {
				return false
			}
		}
		return true
	}

	// 若 prev[0] 仍是当前 row 0，视为未滚动
	if rowEqualsCurrent(prev[0], 0) {
		return
	}

	// 查找最小的 N (1..rows-1) 使得 prev[N] == 当前 row 0
	scrolled := 0
	for n := 1; n < len(prev) && n < rows; n++ {
		if rowEqualsCurrent(prev[n], 0) {
			scrolled = n
			break
		}
	}
	if scrolled == 0 {
		return
	}

	e.mu.Lock()
	sb := e.scrollback
	e.mu.Unlock()
	if sb == nil {
		return
	}
	for i := 0; i < scrolled; i++ {
		sb.PushLine(prev[i])
	}
}

// scanBracketedPasteMode 扫描数据流中的 DECSET 2004 / DECRST 2004 序列。
//   - 启用: ESC [ ? 2004 h
//   - 禁用: ESC [ ? 2004 l
//
// vt10x 不识别 mode 2004，因此需要在外部拦截。
func (e *Emulator) scanBracketedPasteMode(data []byte) {
	// 查找 ESC [ ? 2004 h / l
	for i := 0; i+7 <= len(data); i++ {
		if data[i] != 0x1b || data[i+1] != '[' || data[i+2] != '?' {
			continue
		}
		// 找到 ESC [ ? 后，读取数字直到 h 或 l
		j := i + 3
		for j < len(data) && data[j] >= '0' && data[j] <= '9' {
			j++
		}
		if j >= len(data) {
			continue
		}
		if data[j] == 'h' || data[j] == 'l' {
			// 解析数字
			num := 0
			for k := i + 3; k < j; k++ {
				num = num*10 + int(data[k]-'0')
			}
			if num == 2004 {
				e.mu.Lock()
				if data[j] == 'h' {
					e.bracketedPaste = true
				} else {
					e.bracketedPaste = false
				}
				e.mu.Unlock()
			}
		}
	}
}

// Resize 调整终端尺寸。vt10x 的 Resize 内部会加锁。
func (e *Emulator) Resize(cols, rows int) {
	if cols <= 0 || rows <= 0 {
		return
	}
	e.term.Resize(cols, rows)
	e.mu.Lock()
	e.changed = true
	e.mu.Unlock()
}

// Cell 返回指定位置的字符 glyph。
// 调用方必须先调用 Lock 持有锁，读取完毕后调用 Unlock。
func (e *Emulator) Cell(x, y int) vt10x.Glyph {
	return e.term.Cell(x, y)
}

// Cursor 返回当前光标位置与属性。调用方必须持有 Lock。
func (e *Emulator) Cursor() vt10x.Cursor {
	return e.term.Cursor()
}

// Size 返回当前终端的列数和行数。
// 仅读取 int 值，无需加锁（int 读取在主流架构上是原子的）。
func (e *Emulator) Size() (int, int) {
	return e.term.Size()
}

// Lock 锁定终端状态，用于批量读取。
// 在读取多个 Cell 或 Cursor 时应使用 Lock/Unlock 包裹，
// 避免读到中间状态。内部直接调用 vt10x 的 Lock。
func (e *Emulator) Lock() {
	e.term.Lock()
}

// Unlock 解锁终端状态并重置脏标记。
// vt10x 的 Unlock 会重置内部的 dirty 数组，这里同时重置 changed 标志。
func (e *Emulator) Unlock() {
	e.mu.Lock()
	e.changed = false
	e.mu.Unlock()
	e.term.Unlock()
}

// Mode 返回当前终端模式标志。调用方必须持有 Lock。
func (e *Emulator) Mode() vt10x.ModeFlag {
	return e.term.Mode()
}

// IsAltScreen 返回是否处于备用屏幕模式（vim/htop 等全屏程序）。
// 该方法内部会加锁，不要在 Lock/Unlock 区间内调用。
func (e *Emulator) IsAltScreen() bool {
	e.Lock()
	defer e.Unlock()
	return e.term.Mode()&vt10x.ModeAltScreen != 0
}

// Changed 返回自上次 Unlock 以来是否有数据写入。
// UI 渲染循环可据此判断是否需要刷新。
func (e *Emulator) Changed() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.changed
}

// CursorVisible 返回光标是否可见。调用方必须持有 Lock。
func (e *Emulator) CursorVisible() bool {
	return e.term.CursorVisible()
}

// BracketedPaste 返回当前是否启用了 Bracketed Paste Mode (DECSET 2004)。
// 该方法内部会加锁，不要在 Lock/Unlock 区间内调用。
func (e *Emulator) BracketedPaste() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.bracketedPaste
}

// SetBracketedPaste 直接设置 Bracketed Paste Mode 状态。
// 通常仅在测试或重置终端时使用；正常情况下状态由 DECSET/DECRST 序列驱动。
func (e *Emulator) SetBracketedPaste(enabled bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.bracketedPaste = enabled
}

// MouseTracking 返回当前是否启用了任意鼠标跟踪模式。
// vt10x 支持 ModeMouseButton / ModeMouseMotion / ModeMouseMany / ModeMouseX10，
// 任一模式启用即视为鼠标跟踪已激活。
// 该方法内部会加锁，不要在 Lock/Unlock 区间内调用。
func (e *Emulator) MouseTracking() bool {
	e.Lock()
	defer e.Unlock()
	return e.term.Mode()&vt10x.ModeMouseMask != 0
}

// MouseSgr 返回是否启用了 SGR 鼠标编码（DECSET 1006）。
// 该方法内部会加锁，不要在 Lock/Unlock 区间内调用。
func (e *Emulator) MouseSgr() bool {
	e.Lock()
	defer e.Unlock()
	return e.term.Mode()&vt10x.ModeMouseSgr != 0
}
