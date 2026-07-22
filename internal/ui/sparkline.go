// Package ui 提供 Meatshell 的 Fyne 界面组件，包括主题、标签栏、
// 侧边栏、终端视图、SFTP 浏览器、隧道面板和快捷命令栏。
package ui

import (
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

// Sparkline 是一个迷你折线图组件，显示最近 N 个数据点的趋势。
// 使用滑动窗口保留数据，自动缩放 Y 轴。
type Sparkline struct {
	widget.BaseWidget
	values []float64
	color  color.Color
	max    int     // 最大数据点数
	minVal float64 // 当前最小值（用于 Y 轴缩放）
	maxVal float64 // 当前最大值
}

// NewSparkline 创建一个指定颜色和最大数据点数的迷你折线图
func NewSparkline(c color.Color, max int) *Sparkline {
	if max <= 0 {
		max = 30
	}
	s := &Sparkline{
		values: make([]float64, 0, max),
		color:  c,
		max:    max,
		minVal: math.MaxFloat64,
		maxVal: -math.MaxFloat64,
	}
	s.ExtendBaseWidget(s)
	return s
}

// Push 添加一个新数据点，超出窗口时丢弃最旧的值
func (s *Sparkline) Push(val float64) {
	s.values = append(s.values, val)
	if len(s.values) > s.max {
		s.values = s.values[len(s.values)-s.max:]
	}
	// 重新计算缩放范围
	s.minVal = math.MaxFloat64
	s.maxVal = -math.MaxFloat64
	for _, v := range s.values {
		if v < s.minVal {
			s.minVal = v
		}
		if v > s.maxVal {
			s.maxVal = v
		}
	}
	s.Refresh()
}

// CreateRenderer 实现 fyne.Widget 接口
func (s *Sparkline) CreateRenderer() fyne.WidgetRenderer {
	r := &sparklineRenderer{
		spark: s,
		lines: make([]*canvas.Line, 0, s.max),
	}
	return r
}

// sparklineRenderer 是 Sparkline 的渲染器
type sparklineRenderer struct {
	spark     *Sparkline
	lines     []*canvas.Line
	objects   []fyne.CanvasObject
	lastCount int
}

func (r *sparklineRenderer) Layout(size fyne.Size) {
	r.rebuildLines()
	r.updateLines(size)
}

func (r *sparklineRenderer) MinSize() fyne.Size {
	return fyne.NewSize(40, 20)
}

func (r *sparklineRenderer) Refresh() {
	size := r.spark.Size()
	if size.Width <= 0 || size.Height <= 0 {
		return
	}
	r.rebuildLines()
	r.updateLines(size)
	for _, l := range r.lines {
		canvas.Refresh(l)
	}
}

func (r *sparklineRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *sparklineRenderer) Destroy() {}

// rebuildLines 根据数据点数量重建线段
func (r *sparklineRenderer) rebuildLines() {
	need := len(r.spark.values) - 1
	if need < 0 {
		need = 0
	}
	// 如果线段数量匹配，无需重建
	if len(r.lines) == need {
		return
	}
	// 重建线段
	r.lines = make([]*canvas.Line, need)
	r.objects = make([]fyne.CanvasObject, need)
	for i := range r.lines {
		l := canvas.NewLine(r.spark.color)
		l.StrokeWidth = 1.5
		r.lines[i] = l
		r.objects[i] = l
	}
	r.lastCount = len(r.spark.values)
}

// updateLines 根据数据计算每条线段的位置
func (r *sparklineRenderer) updateLines(size fyne.Size) {
	n := len(r.spark.values)
	if n < 2 || len(r.lines) == 0 {
		return
	}
	w := size.Width
	h := size.Height
	valRange := r.spark.maxVal - r.spark.minVal
	if valRange < 0.001 {
		valRange = 1
	}
	for i := 0; i < n-1 && i < len(r.lines); i++ {
		x1 := float32(i) * w / float32(n-1)
		x2 := float32(i+1) * w / float32(n-1)
		// Y 轴翻转：值越大 Y 越小（屏幕坐标）
		y1 := h - float32((r.spark.values[i]-r.spark.minVal)/valRange)*h
		y2 := h - float32((r.spark.values[i+1]-r.spark.minVal)/valRange)*h
		r.lines[i].Position1 = fyne.NewPos(x1, y1)
		r.lines[i].Position2 = fyne.NewPos(x2, y2)
	}
}
