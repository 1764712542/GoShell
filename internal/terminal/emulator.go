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

	mu      sync.Mutex // 仅保护 changed
	changed bool       // 是否有更新需要重绘

	// bracketedPaste 跟踪 DECSET 2004（Bracketed Paste Mode）。
	// vt10x 库本身不解析 mode 2004，因此由 Emulator 自行维护。
	bracketedPaste bool
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
func (e *Emulator) Write(data []byte) (int, error) {
	// 在写入前扫描 mode 2004 控制序列
	e.scanBracketedPasteMode(data)
	n, err := e.term.Write(data)
	if n > 0 {
		e.mu.Lock()
		e.changed = true
		e.mu.Unlock()
	}
	return n, err
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
