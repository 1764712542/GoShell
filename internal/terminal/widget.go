package terminal

import (
	"fmt"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"github.com/hinshun/vt10x"
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

	// bracketedPaste 控制是否在粘贴剪贴板内容时包裹
	// ESC [200~ ... ESC [201~ 序列。具体是否启用取决于远端程序
	// 是否发送了 DECSET 2004，由 Emulator 跟踪；此处仅作为
	// 用户侧开关，与 Emulator.bracketedPaste 配合使用。
	bracketedPaste bool

	// mouseTracking 由 Emulator 的鼠标模式派生，
	// 当任意鼠标跟踪模式启用时为 true，鼠标事件会被编码为
	// SGR 鼠标序列发送给远端程序。
	mouseTracking bool

	// currentMouseButton 记录当前按下的鼠标按钮，
	// 用于在 MouseMoved 时判断是否需要发送拖动事件。
	currentMouseButton desktop.MouseButton

	// lastMouseCell 记录上次鼠标所在的单元格坐标，
	// 用于在 MouseMoved 时判断是否需要发送 motion 事件。
	lastMouseCellX int
	lastMouseCellY int

	// hyperlinks 管理终端输出中的 OSC 8 超链接。
	hyperlinks *HyperlinkManager

	// 文本选择
	selecting   bool
	selectStart fyne.Position
	selectEnd   fyne.Position

	// 回滚缓冲区
	scrollback   *Scrollback
	scrollOffset int // 滚动偏移（0=底部/当前屏幕，>0=向上滚动）

	// 搜索栏
	searchBar     *SearchBar
	searchVisible bool
	// searchMatches 记录当前查询在 scrollback+screen 中的匹配位置，
	// 每个元素为 (lineIndex, colStart, colEnd)，供渲染高亮使用。
	searchMatches []searchMatch
	searchQuery   string
	// searchCurrent 指向当前高亮的匹配项在 searchMatches 中的下标。
	searchCurrent int
}

// searchMatch 描述一个搜索匹配的位置。
// line 为相对于"scrollback 末尾 + 屏幕首行"起算的绝对行号，
// col 为该行内的字符列起始位置。
type searchMatch struct {
	line int
	col  int
	end  int
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
		hyperlinks: NewHyperlinkManager(),
		scrollback: NewScrollback(10000),
	}
	// 将 scrollback 注入 emulator，使其在 Write 中能捕获滚出的行。
	// 这是修复"正常滚动时 scrollback 从不填充"bug 的关键链接：
	// emulator 的 Write 方法会对比写入前后屏幕内容，把滚出顶部的行压入 scrollback。
	emu.SetScrollback(t.scrollback)
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

	// 检查 Ctrl+Shift+C (复制) 和 Ctrl+Shift+V (粘贴)
	drv, ok := fyne.CurrentApp().Driver().(desktop.Driver)
	if ok && drv.CurrentKeyModifiers()&fyne.KeyModifierControl != 0 && drv.CurrentKeyModifiers()&fyne.KeyModifierShift != 0 {
		switch key.Name {
		case fyne.KeyC:
			t.CopySelection()
			return
		case fyne.KeyV:
			t.pasteFromClipboard()
			return
		}
	}

	// Ctrl 组合键
	if t.ctrlPressed() {
		// Ctrl+V (0x16) 触发粘贴
		if key.Name == fyne.KeyV {
			t.pasteFromClipboard()
			return
		}
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
// 当启用了 Bracketed Paste Mode 且检测到 Ctrl+V 控制字符 (0x16) 时，
// 从系统剪贴板读取内容并用 ESC [200~ ... ESC [201~ 包裹后发送。
func (t *TerminalWidget) TypedRune(r rune) {
	if t.inputCB == nil {
		return
	}
	// 0x16 是 Ctrl+V 的控制字符 (SYN)，在 bracketed paste 模式下触发粘贴
	if t.bracketedPaste && r == 0x16 {
		t.pasteFromClipboard()
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
	// 尺寸变化前捕获当前屏幕到回滚缓冲区
	t.CaptureScreenToScrollback()
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

// SetBracketedPaste 启用/禁用 Bracketed Paste Mode。
// 启用后，TypedRune 收到 Ctrl+V (0x16) 时会从系统剪贴板读取内容，
// 并用 ESC [200~ ... ESC [201~ 包裹后发送到远端。
func (t *TerminalWidget) SetBracketedPaste(enabled bool) {
	t.bracketedPaste = enabled
}

// BracketedPaste 返回 Bracketed Paste Mode 是否启用。
func (t *TerminalWidget) BracketedPaste() bool {
	return t.bracketedPaste
}

// pasteFromClipboard 从系统剪贴板读取内容并发送到远端。
// 当 Emulator 跟踪到远端程序启用了 mode 2004 时，会用
// ESC [200~ ... ESC [201~ 包裹剪贴板内容；否则直接发送原始内容。
func (t *TerminalWidget) pasteFromClipboard() {
	if t.inputCB == nil {
		return
	}

	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	driver := app.Driver()
	if driver == nil {
		return
	}
	wins := driver.AllWindows()
	if len(wins) == 0 {
		return
	}
	clip := wins[0].Clipboard()
	if clip == nil {
		return
	}
	content := clip.Content()
	if content == "" {
		return
	}

	// 仅当远端程序启用了 mode 2004 时才包裹 bracket 序列
	if t.emulator != nil && t.emulator.BracketedPaste() {
		wrapped := fmt.Sprintf("\x1b[200~%s\x1b[201~", content)
		t.inputCB([]byte(wrapped))
		return
	}
	t.inputCB([]byte(content))
}

// CopySelection 将当前选中的文本复制到系统剪贴板。
// 若没有正在选择或选中内容为空则不做任何操作。
func (t *TerminalWidget) CopySelection() {
	if !t.selecting {
		return
	}
	text := t.getSelectedText()
	if text == "" {
		return
	}
	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	driver := app.Driver()
	if driver == nil {
		return
	}
	wins := driver.AllWindows()
	if len(wins) == 0 {
		return
	}
	clip := wins[0].Clipboard()
	if clip == nil {
		return
	}
	clip.SetContent(text)
}

// getSelectedText 读取 selectStart 到 selectEnd 之间的单元格文本。
// 起止点会先做归一化（确保 start <= end），跨行时用 '\n' 分隔。
func (t *TerminalWidget) getSelectedText() string {
	if t.emulator == nil {
		return ""
	}
	startX, startY := t.positionToCell(t.selectStart)
	endX, endY := t.positionToCell(t.selectEnd)
	// 归一化：确保 start < end
	if startY > endY || (startY == endY && startX > endX) {
		startX, startY, endX, endY = endX, endY, startX, startY
	}
	var sb strings.Builder
	t.emulator.Lock()
	defer t.emulator.Unlock()
	for y := startY; y <= endY; y++ {
		xStart := 0
		xEnd := t.cols - 1
		if y == startY {
			xStart = startX
		}
		if y == endY {
			xEnd = endX
		}
		for x := xStart; x <= xEnd; x++ {
			g := t.emulator.Cell(x, y)
			ch := g.Char
			if ch == 0 {
				ch = ' '
			}
			sb.WriteRune(ch)
		}
		if y < endY {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// SetMouseTracking 手动启用/禁用鼠标跟踪（覆盖 Emulator 派生的状态）。
// 通常鼠标跟踪状态由 Emulator 的鼠标模式自动派生，调用此方法可强制覆盖。
func (t *TerminalWidget) SetMouseTracking(enabled bool) {
	t.mouseTracking = enabled
}

// MouseTracking 返回鼠标跟踪是否启用。
// 优先返回手动设置的值，否则根据 Emulator 的鼠标模式派生。
func (t *TerminalWidget) MouseTracking() bool {
	if t.mouseTracking {
		return true
	}
	if t.emulator != nil {
		return t.emulator.MouseTracking()
	}
	return false
}

// MouseIn 实现 desktop.Hoverable 接口。
// 当鼠标跟踪启用时，将鼠标进入事件编码为 SGR 鼠标序列发送。
func (t *TerminalWidget) MouseIn(evt *desktop.MouseEvent) {
	t.handleMouseEvent(evt, false)
}

// MouseMoved 实现 desktop.Hoverable 接口。
// 当鼠标跟踪启用且鼠标移动到新的单元格时，发送 motion 事件。
// 仅在鼠标按钮按下时发送拖动事件（button 32），或在 ModeMouseMany 下
// 持续发送 motion 事件。
// 当未启用鼠标跟踪且正在选择文本时，更新选择终点并刷新。
func (t *TerminalWidget) MouseMoved(evt *desktop.MouseEvent) {
	if t.selecting {
		t.selectEnd = evt.Position
		t.Refresh()
		return
	}
	t.handleMouseEvent(evt, true)
}

// MouseOut 实现 desktop.Hoverable 接口。
func (t *TerminalWidget) MouseOut() {
	t.currentMouseButton = 0
	t.lastMouseCellX = -1
	t.lastMouseCellY = -1
}

// Tapped 实现 fyne.Tappable 接口。
// 当鼠标跟踪启用时，将点击事件作为鼠标按钮事件发送；
// 否则检查点击位置是否落在 OSC 8 超链接上，若是则打开 URL。
// 无论哪种模式，都先获取焦点以确保键盘输入能到达终端。
func (t *TerminalWidget) Tapped(evt *fyne.PointEvent) {
	t.requestFocus()
	if t.MouseTracking() {
		// 鼠标跟踪模式下，点击事件由 MouseDown/MouseUp 处理
		return
	}
	// 检查是否点击了超链接
	if t.hyperlinks == nil {
		return
	}
	x, y := t.positionToCell(evt.Position)
	if url := t.hyperlinks.URLAt(x, y); url != "" {
		OpenURL(url)
	}
}

// Scrolled 实现 fyne.Scrollable 接口。
// 当鼠标跟踪启用时，将滚轮事件编码为 SGR 鼠标序列（button 64/65）发送。
// 否则滚动回滚缓冲区：向上滚动增加偏移，向下滚动减少偏移。
func (t *TerminalWidget) Scrolled(evt *fyne.ScrollEvent) {
	if t.MouseTracking() {
		if t.inputCB == nil {
			return
		}
		x, y := t.positionToCell(evt.Position)
		// 向上滚动 -> button 64，向下滚动 -> button 65
		button := 64
		if evt.Scrolled.DY < 0 {
			button = 65
		}
		t.sendSGRMouse(button, x, y, true)
		return
	}
	// 滚动回滚缓冲区
	if evt.Scrolled.DY > 0 {
		t.scrollOffsetUp(3) // 每个滚轮刻度向上滚动 3 行
	} else {
		t.scrollOffsetDown(3)
	}
}

// MouseDown 实现 desktop.Mouseable 接口。
// 当鼠标跟踪启用时，将鼠标按下事件编码为 SGR 鼠标序列发送。
// 否则在左键按下时开始文本选择。
// 无论哪种模式，都先获取焦点以确保键盘输入能到达终端
// （修复打开 SFTP 面板后点击终端无法输入的问题）。
func (t *TerminalWidget) MouseDown(evt *desktop.MouseEvent) {
	t.requestFocus()
	if t.MouseTracking() {
		t.currentMouseButton = evt.Button
		t.handleMouseEvent(evt, false)
		return
	}
	// 非鼠标跟踪模式：左键按下开始文本选择
	if evt.Button == desktop.MouseButtonPrimary {
		t.selecting = true
		t.selectStart = evt.Position
		t.selectEnd = evt.Position
	}
}

// requestFocus 请求当前 canvas 将焦点设置到本 widget。
// 在 MouseDown 和 Tapped 中调用，确保用户点击终端后键盘输入能到达终端。
// 这对修复"打开 SFTP 等面板后点击终端无法输入"的问题至关重要。
func (t *TerminalWidget) requestFocus() {
	c := fyne.CurrentApp().Driver().CanvasForObject(t)
	if c != nil {
		c.Focus(t)
	}
}

// MouseUp 实现 desktop.Mouseable 接口。
// 当鼠标跟踪启用时，将鼠标释放事件编码为 SGR 鼠标序列发送（使用 'm' 而非 'M'）。
// 否则在左键释放时结束文本选择并自动复制到剪贴板。
func (t *TerminalWidget) MouseUp(evt *desktop.MouseEvent) {
	if t.MouseTracking() {
		x, y := t.positionToCell(evt.Position)
		button := mouseButtonToSGR(evt.Button)
		// 释放事件使用 'm' 后缀
		t.sendSGRMouse(button, x, y, false)
		t.currentMouseButton = 0
		return
	}
	// 结束文本选择
	if t.selecting && evt.Button == desktop.MouseButtonPrimary {
		t.selectEnd = evt.Position
		// 选中区域非空时自动复制到剪贴板
		if t.selectStart != t.selectEnd {
			t.CopySelection()
		}
		t.selecting = false
	}
}

// handleMouseEvent 统一处理鼠标移动/进入事件。
// isMove 为 true 表示这是 MouseMoved 事件。
func (t *TerminalWidget) handleMouseEvent(evt *desktop.MouseEvent, isMove bool) {
	if !t.MouseTracking() {
		return
	}
	if t.inputCB == nil {
		return
	}
	x, y := t.positionToCell(evt.Position)

	if isMove {
		// 仅在单元格变化时发送，避免洪水
		if x == t.lastMouseCellX && y == t.lastMouseCellY {
			return
		}
		t.lastMouseCellX = x
		t.lastMouseCellY = y

		// motion 事件：仅在按下按钮时发送 drag（button 32），
		// 或在 ModeMouseMany 下持续发送 motion 事件。
		if t.currentMouseButton != 0 {
			// 拖动：在原按钮基础上加 32
			button := mouseButtonToSGR(t.currentMouseButton) + 32
			t.sendSGRMouse(button, x, y, true)
		} else if t.emulator != nil {
			// ModeMouseMany 下即使无按钮按下也发送 motion 事件
			t.emulator.Lock()
			many := t.emulator.Mode()&vt10x.ModeMouseMany != 0
			t.emulator.Unlock()
			if many {
				t.sendSGRMouse(35, x, y, true)
			}
		}
		return
	}

	// MouseIn：发送按下事件
	button := mouseButtonToSGR(evt.Button)
	t.sendSGRMouse(button, x, y, true)
}

// sendSGRMouse 发送 SGR 鼠标序列: ESC [ < button ; x ; y M/m
// pressed 为 true 使用 'M'（按下），false 使用 'm'（释放）。
func (t *TerminalWidget) sendSGRMouse(button, x, y int, pressed bool) {
	if t.inputCB == nil {
		return
	}
	// 终端坐标从 1 开始
	suffix := "M"
	if !pressed {
		suffix = "m"
	}
	seq := fmt.Sprintf("\x1b[<%d;%d;%d%s", button, x+1, y+1, suffix)
	t.inputCB([]byte(seq))
}

// positionToCell 将像素坐标转换为终端单元格坐标 (0-based)。
func (t *TerminalWidget) positionToCell(pos fyne.Position) (int, int) {
	cw := t.charWidth
	ch := t.charHeight
	if cw <= 0 || ch <= 0 {
		return 0, 0
	}
	x := int(pos.X / cw)
	y := int(pos.Y / ch)
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if x >= t.cols {
		x = t.cols - 1
	}
	if y >= t.rows {
		y = t.rows - 1
	}
	return x, y
}

// mouseButtonToSGR 将 Fyne 鼠标按钮映射为 SGR 鼠标按钮编码。
//   - 左键 (Primary)   -> 0
//   - 中键 (Tertiary)  -> 1
//   - 右键 (Secondary) -> 2
func mouseButtonToSGR(b desktop.MouseButton) int {
	switch b {
	case desktop.MouseButtonPrimary:
		return 0
	case desktop.MouseButtonTertiary:
		return 1
	case desktop.MouseButtonSecondary:
		return 2
	default:
		return 0
	}
}

// Cursor 实现 desktop.Cursorable 接口。
// 当鼠标悬停在 OSC 8 超链接单元格上时返回 PointerCursor。
func (t *TerminalWidget) Cursor() desktop.Cursor {
	// 在没有鼠标跟踪时，悬停在超链接上显示手型光标
	if !t.MouseTracking() && t.hyperlinks != nil {
		// 由于无法在 Cursor() 中获取鼠标位置，
		// 实际的指针切换由 MouseMoved 通过 SetCursor 实现。
		// 这里返回默认光标，MouseMoved 会动态切换。
	}
	return desktop.DefaultCursor
}

// Hyperlinks 返回超链接管理器（供外部刷新/查询）。
func (t *TerminalWidget) Hyperlinks() *HyperlinkManager {
	return t.hyperlinks
}

// Cols 返回当前终端列数。
func (t *TerminalWidget) Cols() int { return t.cols }

// Rows 返回当前终端行数。
func (t *TerminalWidget) Rows() int { return t.rows }

// Emulator 返回关联的 Emulator 实例。
func (t *TerminalWidget) Emulator() *Emulator { return t.emulator }

// scrollOffsetUp 向上滚动（查看更早的历史输出）指定行数。
func (t *TerminalWidget) scrollOffsetUp(lines int) {
	if lines <= 0 {
		return
	}
	t.scrollOffset += lines
	max := 0
	if t.scrollback != nil {
		max = t.scrollback.Count()
	}
	if t.scrollOffset > max {
		t.scrollOffset = max
	}
	t.Refresh()
}

// scrollOffsetDown 向下滚动（回到更近的输出）指定行数。
// 当偏移归零时回到实时屏幕。
func (t *TerminalWidget) scrollOffsetDown(lines int) {
	if lines <= 0 {
		return
	}
	t.scrollOffset -= lines
	if t.scrollOffset < 0 {
		t.scrollOffset = 0
	}
	t.Refresh()
}

// ScrollOffset 返回当前回滚偏移量（0 表示处于实时屏幕底部）。
func (t *TerminalWidget) ScrollOffset() int { return t.scrollOffset }

// Scrollback 返回关联的回滚缓冲区。
func (t *TerminalWidget) Scrollback() *Scrollback { return t.scrollback }

// ResetScrollOffset 将回滚偏移重置为 0（回到实时屏幕）。
func (t *TerminalWidget) ResetScrollOffset() {
	if t.scrollOffset != 0 {
		t.scrollOffset = 0
		t.Refresh()
	}
}

// CaptureScreenToScrollback 捕获当前可见屏幕（含完整属性）并压入回滚缓冲区。
// 通常在终端 resize 或清屏前调用，以保留历史输出。
// 使用 CaptureScreenCells 而非 CaptureScreen，以保留每个 cell 的颜色和样式。
func (t *TerminalWidget) CaptureScreenToScrollback() {
	if t.emulator == nil || t.scrollback == nil {
		return
	}
	lines := CaptureScreenCells(t.emulator.term, t.cols, t.rows)
	for _, line := range lines {
		t.scrollback.PushLine(line)
	}
}

// Selecting 返回当前是否正在进行文本选择。
func (t *TerminalWidget) Selecting() bool { return t.selecting }

// SelectStart 返回选择起点位置。
func (t *TerminalWidget) SelectStart() fyne.Position { return t.selectStart }

// SelectEnd 返回选择终点位置。
func (t *TerminalWidget) SelectEnd() fyne.Position { return t.selectEnd }

// SearchBar 返回关联的搜索栏（可能为 nil）。
func (t *TerminalWidget) SearchBar() *SearchBar { return t.searchBar }

// SearchVisible 返回搜索栏是否可见。
func (t *TerminalWidget) SearchVisible() bool { return t.searchVisible }

// SetSearchBar 注入外部创建的搜索栏实例，并绑定搜索回调。
func (t *TerminalWidget) SetSearchBar(sb *SearchBar) {
	t.searchBar = sb
}

// ToggleSearch 切换搜索栏的显示状态。
// 显示时若搜索栏未创建则按需创建（需要外部已通过 SetSearchBar 注入）。
func (t *TerminalWidget) ToggleSearch() {
	t.searchVisible = !t.searchVisible
	if t.searchVisible && t.searchBar != nil {
		t.searchBar.Focus()
	}
	t.Refresh()
}

// SetSearchVisible 直接设置搜索栏的可见性。
func (t *TerminalWidget) SetSearchVisible(visible bool) {
	t.searchVisible = visible
	if visible && t.searchBar != nil {
		t.searchBar.Focus()
	}
	t.Refresh()
}

// Search 在回滚缓冲区 + 当前屏幕中搜索 query。
// direction 为 1 表示跳到下一个匹配，-1 表示上一个，0 表示重新搜索。
// 找到匹配后会滚动到对应位置并刷新高亮。
func (t *TerminalWidget) Search(query string, direction int) {
	if query == "" {
		t.searchQuery = ""
		t.searchMatches = nil
		t.searchCurrent = 0
		if t.searchBar != nil {
			t.searchBar.SetResult("")
		}
		t.Refresh()
		return
	}

	// 新查询时重新计算所有匹配
	if query != t.searchQuery {
		t.searchQuery = query
		t.searchMatches = t.findAllMatches(query)
		t.searchCurrent = 0
	} else if len(t.searchMatches) > 0 {
		// 同一查询，根据方向切换当前匹配
		switch direction {
		case 1:
			t.searchCurrent = (t.searchCurrent + 1) % len(t.searchMatches)
		case -1:
			t.searchCurrent = (t.searchCurrent - 1 + len(t.searchMatches)) % len(t.searchMatches)
		}
	}

	if t.searchBar != nil {
		if len(t.searchMatches) == 0 {
			t.searchBar.SetResult("0/0")
		} else {
			t.searchBar.SetResult(fmt.Sprintf("%d/%d", t.searchCurrent+1, len(t.searchMatches)))
		}
	}

	// 滚动到当前匹配
	if len(t.searchMatches) > 0 {
		t.scrollToMatch(t.searchMatches[t.searchCurrent])
	}
	t.Refresh()
}

// findAllMatches 在回滚缓冲区 + 当前屏幕中查找所有匹配位置。
// 返回的 searchMatch.line 为绝对行号：0 表示回滚缓冲区第一行，
// 回滚缓冲区之后紧跟当前屏幕行。
func (t *TerminalWidget) findAllMatches(query string) []searchMatch {
	if query == "" {
		return nil
	}
	var matches []searchMatch
	// 收集回滚缓冲区行（ScrollbackLine -> 纯文本）
	var allLines []string
	if t.scrollback != nil {
		for _, line := range t.scrollback.Lines() {
			allLines = append(allLines, line.String())
		}
	}
	// 收集当前屏幕行
	if t.emulator != nil {
		screenLines := CaptureScreen(t.emulator.term, t.cols, t.rows)
		allLines = append(allLines, screenLines...)
	}
	q := strings.ToLower(query)
	for lineIdx, line := range allLines {
		lower := strings.ToLower(line)
		start := 0
		for {
			pos := strings.Index(lower[start:], q)
			if pos < 0 {
				break
			}
			col := start + pos
			matches = append(matches, searchMatch{
				line: lineIdx,
				col:  col,
				end:  col + len(query),
			})
			start = col + 1
			if start >= len(lower) {
				break
			}
		}
	}
	return matches
}

// scrollToMatch 滚动视图使指定匹配可见。
func (t *TerminalWidget) scrollToMatch(m searchMatch) {
	if t.scrollback == nil {
		return
	}
	sbCount := t.scrollback.Count()
	// 匹配所在行相对于屏幕顶部的偏移
	// line < sbCount 表示在回滚缓冲区中，否则在当前屏幕内
	if m.line < sbCount {
		// 该匹配在回滚缓冲区中，需要向上滚动
		// 屏幕顶部应位于回滚缓冲区的 (m.line - scrollOffset) 位置
		// 即 scrollOffset = sbCount - m.line
		t.scrollOffset = sbCount - m.line
		if t.scrollOffset < 0 {
			t.scrollOffset = 0
		}
	} else {
		// 匹配在当前屏幕内，回到实时屏幕
		t.scrollOffset = 0
	}
}
