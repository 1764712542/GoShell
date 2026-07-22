package terminal

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// TerminalWidget 是终端渲染的核心 Fyne Widget。
// 它持有 Emulator 引用，通过 CreateRenderer 创建渲染器，
// 并通过 TypedKey/TypedRune 接收键盘输入转发到 SSH。
type TerminalWidget struct {
	widget.BaseWidget

	emulator   *Emulator
	inputCB    func([]byte) // 键盘输入回调（发送到 SSH stdin）
	cols, rows int
	fontSize   float32
	charWidth  float32 // 字符宽度（像素，由 Layout 计算）
	charHeight float32 // 字符高度（像素，由 Layout 计算）
	fg, bg     color.Color
	termColors [16]color.Color // ANSI 16 色调色板
	refresh    chan struct{}   // 刷新触发信号
}

// NewTerminalWidget 创建一个终端 Widget。
// inputCB 用于将用户的键盘输入转发到 SSH 会话的 stdin。
func NewTerminalWidget(emu *Emulator, inputCB func([]byte)) *TerminalWidget {
	cols, rows := emu.Size()
	t := &TerminalWidget{
		emulator:   emu,
		inputCB:    inputCB,
		cols:       cols,
		rows:       rows,
		fontSize:   14,
		fg:         color.RGBA{R: 0xD3, G: 0xD7, B: 0xCF, A: 0xff},
		bg:         color.RGBA{R: 0x1E, G: 0x1E, B: 0x1E, A: 0xff},
		termColors: DefaultTermColors,
		refresh:    make(chan struct{}, 1),
	}
	t.ExtendBaseWidget(t)
	return t
}

// CreateRenderer 实现 fyne.Widget 接口，返回终端渲染器。
func (t *TerminalWidget) CreateRenderer() fyne.WidgetRenderer {
	return newTerminalRenderer(t)
}

// TypedKey 处理非字符按键事件（方向键、功能键等）。
// 实现 fyne.Focusable 接口。
// 注意：普通可打印字符由 TypedRune 处理，这里只处理特殊键和 Ctrl 组合键。
func (t *TerminalWidget) TypedKey(key *fyne.KeyEvent) {
	if t.inputCB == nil {
		return
	}
	// 检查 Ctrl 修饰键是否按下
	if t.ctrlPressed() {
		if data := ctrlKeyToANSI(key.Name); data != nil {
			t.inputCB(data)
			return
		}
	}
	// 其他特殊键（方向键、功能键等）
	if data := keyToANSI(key.Name); data != nil {
		t.inputCB(data)
	}
}

// ctrlPressed 返回 Ctrl 修饰键当前是否被按下。
// 通过 Fyne desktop driver 的 CurrentKeyModifiers 获取。
func (t *TerminalWidget) ctrlPressed() bool {
	drv, ok := fyne.CurrentApp().Driver().(desktop.Driver)
	if !ok {
		return false
	}
	return drv.CurrentKeyModifiers()&fyne.KeyModifierControl != 0
}

// TypedRune 处理字符输入。实现 fyne.Focusable 接口。
func (t *TerminalWidget) TypedRune(r rune) {
	if t.inputCB == nil {
		return
	}
	t.inputCB([]byte(string(r)))
}

// FocusGained 实现 fyne.Focusable 接口。
func (t *TerminalWidget) FocusGained() {}

// FocusLost 实现 fyne.Focusable 接口。
func (t *TerminalWidget) FocusLost() {}

// TriggerRefresh 触发终端重绘。
// SSH 读循环在写入数据到 Emulator 后调用此方法通知 UI 刷新。
// 使用非阻塞式发送，如果已有待处理的刷新信号则跳过。
func (t *TerminalWidget) TriggerRefresh() {
	t.BaseWidget.Refresh()
}

// SetSize 调整终端大小（列数和行数）。
// 同时更新 emulator 和 widget 本身的尺寸。
func (t *TerminalWidget) SetSize(cols, rows int) {
	if cols <= 0 || rows <= 0 {
		return
	}
	if cols == t.cols && rows == t.rows {
		return
	}
	t.cols = cols
	t.rows = rows
	t.emulator.Resize(cols, rows)
	t.Refresh()
}

// SetFontSize 设置终端字体大小。
func (t *TerminalWidget) SetFontSize(size float32) {
	if size <= 0 {
		return
	}
	t.fontSize = size
	t.Refresh()
}

// SetColors 设置默认前景色、背景色和 16 色调色板。
func (t *TerminalWidget) SetColors(fg, bg color.Color, palette [16]color.Color) {
	t.fg = fg
	t.bg = bg
	t.termColors = palette
	t.Refresh()
}

// Cols 返回当前终端列数。
func (t *TerminalWidget) Cols() int { return t.cols }

// Rows 返回当前终端行数。
func (t *TerminalWidget) Rows() int { return t.rows }

// Emulator 返回关联的 Emulator 实例。
func (t *TerminalWidget) Emulator() *Emulator { return t.emulator }
