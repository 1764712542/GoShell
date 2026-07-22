package terminal

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"github.com/hinshun/vt10x"
)

// attrReverse 与 vt10x 内部的反色属性位一致（见 state.go）。
const attrReverse int16 = 1

// terminalRenderer 是 TerminalWidget 的渲染器。
// 每个 cell 由一个 canvas.Rectangle（背景）和一个 canvas.Text（前景字符）组成。
// 光标也使用一个 canvas.Rectangle，叠在最上层。
type terminalRenderer struct {
	widget    *TerminalWidget
	bg        *canvas.Rectangle     // 整体背景
	cursor    *canvas.Rectangle     // 光标
	cellBGs   []*canvas.Rectangle   // 每个 cell 的背景矩形
	cells     []*canvas.Text        // 每个 cell 的字符（与 cellBGs 一一对应）
	objects   []fyne.CanvasObject   // 所有需要绘制的对象
	cellSize  fyne.Size             // 单个字符的尺寸（缓存）
	lastGlyph []vt10x.Glyph         // 上次绘制时的 glyph，用于脏 cell 追踪
}

// newTerminalRenderer 构造一个 TerminalWidget 的渲染器。
func newTerminalRenderer(w *TerminalWidget) *terminalRenderer {
	r := &terminalRenderer{
		widget: w,
		bg:     canvas.NewRectangle(w.bg),
		cursor: canvas.NewRectangle(w.fg),
	}
	r.cursor.Hide()
	r.rebuildCells()
	r.refreshObjects()
	return r
}

// rebuildCells 重新分配 cell 数组并应用默认样式。
func (r *terminalRenderer) rebuildCells() {
	count := r.widget.cols * r.widget.rows
	if count < 0 {
		count = 0
	}
	r.cellBGs = make([]*canvas.Rectangle, count)
	r.cells = make([]*canvas.Text, count)
	r.lastGlyph = make([]vt10x.Glyph, count)
	for i := 0; i < count; i++ {
		bg := canvas.NewRectangle(r.widget.bg)
		bg.Hide() // 默认背景由整体 bg 负责，隐藏以减少绘制
		r.cellBGs[i] = bg

		t := canvas.NewText(" ", r.widget.fg)
		t.TextSize = r.widget.fontSize
		t.TextStyle = fyne.TextStyle{Monospace: true}
		r.cells[i] = t
	}
}

// refreshObjects 重建 objects 切片。
// 顺序：整体背景 -> 每 cell (bgRect, text) -> 光标。
// 绘制层级从底到顶。
func (r *terminalRenderer) refreshObjects() {
	r.objects = r.objects[:0]
	r.objects = append(r.objects, r.bg)
	for i := range r.cells {
		r.objects = append(r.objects, r.cellBGs[i])
		r.objects = append(r.objects, r.cells[i])
	}
	r.objects = append(r.objects, r.cursor)
}

// Layout 计算字符尺寸并布局所有 cell。
func (r *terminalRenderer) Layout(size fyne.Size) {
	r.bg.Resize(size)
	r.bg.Move(fyne.NewPos(0, 0))

	cols := r.widget.cols
	rows := r.widget.rows
	if cols <= 0 || rows <= 0 {
		return
	}

	charW := size.Width / float32(cols)
	charH := size.Height / float32(rows)
	r.cellSize = fyne.NewSize(charW, charH)
	r.widget.charWidth = charW
	r.widget.charHeight = charH

	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			idx := y*cols + x
			if idx >= len(r.cells) {
				break
			}
			pos := fyne.NewPos(float32(x)*charW, float32(y)*charH)
			r.cellBGs[idx].Resize(r.cellSize)
			r.cellBGs[idx].Move(pos)
			r.cells[idx].Resize(r.cellSize)
			r.cells[idx].Move(pos)
		}
	}

	r.cursor.Resize(r.cellSize)
}

// MinSize 返回 widget 的最小尺寸。
func (r *terminalRenderer) MinSize() fyne.Size {
	cols := r.widget.cols
	rows := r.widget.rows
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	return fyne.NewSize(r.widget.fontSize*0.6*float32(cols), r.widget.fontSize*1.2*float32(rows))
}

// Objects 返回所有需要绘制的 CanvasObject。
func (r *terminalRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

// Destroy 在渲染器销毁时调用。
func (r *terminalRenderer) Destroy() {}

// Refresh 重绘所有 cell。
// 通过比较 lastGlyph 与当前 glyph 实现脏 cell 追踪，只刷新变化的 cell。
func (r *terminalRenderer) Refresh() {
	w := r.widget
	emu := w.emulator
	if emu == nil {
		return
	}

	cols := w.cols
	rows := w.rows

	// 尺寸变化时重建 cell 数组
	if len(r.cells) != cols*rows {
		r.rebuildCells()
		r.refreshObjects()
	}

	emu.Lock()
	defer emu.Unlock()

	curCols, curRows := emu.Size()
	if curCols != cols || curRows != rows {
		// emulator 与 widget 尺寸不同步，跳过本次绘制
		return
	}

	cursor := emu.Cursor()
	cursorVisible := emu.CursorVisible()

	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			idx := y*cols + x
			if idx >= len(r.cells) {
				break
			}
			glyph := emu.Cell(x, y)

			// 脏 cell 追踪：只有 glyph 变化时才刷新
			if idx < len(r.lastGlyph) && r.lastGlyph[idx] == glyph {
				continue
			}
			if idx < len(r.lastGlyph) {
				r.lastGlyph[idx] = glyph
			}

			cell := r.cells[idx]
			cellBG := r.cellBGs[idx]

			fg, bg := r.resolveColors(glyph)
			char := glyph.Char
			if char == 0 {
				char = ' '
			}
			cell.Text = string(char)
			cell.Color = fg
			cell.TextSize = w.fontSize
			cell.TextStyle = fyne.TextStyle{Monospace: true}

			// 非默认背景：显示 cell 级背景矩形
			if !colorsEqual(bg, w.bg) {
				cellBG.FillColor = bg
				cellBG.Show()
			} else {
				cellBG.Hide()
			}

			canvas.Refresh(cellBG)
			canvas.Refresh(cell)
		}
	}

	// 光标
	if cursorVisible && cursor.X >= 0 && cursor.X < cols && cursor.Y >= 0 && cursor.Y < rows {
		r.cursor.FillColor = w.fg
		r.cursor.Move(fyne.NewPos(float32(cursor.X)*w.charWidth, float32(cursor.Y)*w.charHeight))
		r.cursor.Resize(r.cellSize)
		r.cursor.Show()
		canvas.Refresh(r.cursor)
	} else {
		r.cursor.Hide()
	}
}

// resolveColors 将 vt10x.Glyph 的 FG/BG 解析为 color.Color。
// DefaultFG/DefaultBG 映射到 widget 的默认前景/背景色。
// 反色属性会交换前景与背景。
func (r *terminalRenderer) resolveColors(g vt10x.Glyph) (fg, bg color.Color) {
	fg = r.glyphColor(g.FG, r.widget.fg)
	bg = r.glyphColor(g.BG, r.widget.bg)
	if g.Mode&attrReverse != 0 {
		fg, bg = bg, fg
	}
	return
}

// glyphColor 把 vt10x.Color 转换为 color.Color。
//   - DefaultFG/DefaultBG/DefaultCursor 映射到 def
//   - 0-15 走 ANSI 16 色调色板
//   - 16-255 走 xterm 256 色编码
//   - 其他视为 RGB（r<<16 | g<<8 | b）
func (r *terminalRenderer) glyphColor(c vt10x.Color, def color.Color) color.Color {
	switch {
	case c == vt10x.DefaultFG, c == vt10x.DefaultBG, c == vt10x.DefaultCursor:
		return def
	case c < 16:
		return r.widget.termColors[int(c)]
	case c < 256:
		return xterm256ToRGBA(uint8(c))
	default:
		return color.RGBA{
			R: uint8(c>>16) & 0xff,
			G: uint8(c>>8) & 0xff,
			B: uint8(c) & 0xff,
			A: 0xff,
		}
	}
}

// xterm256ToRGBA 将 xterm 256 色索引（16-255）转换为 RGBA。
func xterm256ToRGBA(idx uint8) color.Color {
	if idx < 16 {
		return DefaultTermColors[idx]
	}
	if idx < 232 {
		// 216 色立方体：6×6×6
		i := int(idx) - 16
		r := i / 36
		g := (i % 36) / 6
		b := i % 6
		to8 := func(v int) uint8 {
			if v == 0 {
				return 0
			}
			return uint8(55 + v*40)
		}
		return color.RGBA{R: to8(r), G: to8(g), B: to8(b), A: 0xff}
	}
	// 24 级灰阶
	gray := uint8(8 + (int(idx)-232)*10)
	return color.RGBA{R: gray, G: gray, B: gray, A: 0xff}
}

// colorsEqual 比较两个 color.Color 是否相等（基于 RGBA 值）。
func colorsEqual(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == b
	}
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

// DefaultTermColors 是深色主题的 ANSI 16 色调色板。
var DefaultTermColors = [16]color.Color{
	color.RGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xff}, // 0 黑
	color.RGBA{R: 0xCC, G: 0x00, B: 0x00, A: 0xff}, // 1 红
	color.RGBA{R: 0x4E, G: 0x9A, B: 0x06, A: 0xff}, // 2 绿
	color.RGBA{R: 0xC4, G: 0xA0, B: 0x00, A: 0xff}, // 3 黄
	color.RGBA{R: 0x34, G: 0x65, B: 0xA4, A: 0xff}, // 4 蓝
	color.RGBA{R: 0x75, G: 0x50, B: 0x7B, A: 0xff}, // 5 紫
	color.RGBA{R: 0x06, G: 0x98, B: 0x9A, A: 0xff}, // 6 青
	color.RGBA{R: 0xD3, G: 0xD7, B: 0xCF, A: 0xff}, // 7 白
	// 亮色 8-15
	color.RGBA{R: 0x55, G: 0x57, B: 0x53, A: 0xff}, // 8 亮黑
	color.RGBA{R: 0xEF, G: 0x29, B: 0x29, A: 0xff}, // 9 亮红
	color.RGBA{R: 0x8A, G: 0xE2, B: 0x34, A: 0xff}, // 10 亮绿
	color.RGBA{R: 0xFC, G: 0xE9, B: 0x4F, A: 0xff}, // 11 亮黄
	color.RGBA{R: 0x72, G: 0x9F, B: 0xCF, A: 0xff}, // 12 亮蓝
	color.RGBA{R: 0xAD, G: 0x7F, B: 0xA8, A: 0xff}, // 13 亮紫
	color.RGBA{R: 0x34, G: 0xE2, B: 0xE2, A: 0xff}, // 14 亮青
	color.RGBA{R: 0xEE, G: 0xEE, B: 0xEC, A: 0xff}, // 15 亮白
}
