package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// 标签栏颜色
var (
	colorActive   = color.RGBA{R: 0x89, G: 0xb4, B: 0xfa, A: 0x3f}
	colorInactive = color.RGBA{R: 0x31, G: 0x31, B: 0x44, A: 0xff}
)

// TabItem 表示标签栏中的一个标签项
type TabItem struct {
	ID       string
	Title    string
	Closable bool
}

// TabBar 是可水平滚动的标签栏组件，支持添加、关闭和切换标签。
type TabBar struct {
	widget.BaseWidget
	tabs     []*TabItem
	active   int
	onChange func(index int)
	onAdd    func()
	onClose  func(index int)

	scroll *container.Scroll
	hbox   *fyne.Container // 内部水平布局
}

// NewTabBar 创建标签栏
func NewTabBar() *TabBar {
	t := &TabBar{
		tabs:   make([]*TabItem, 0),
		active: -1,
	}
	t.ExtendBaseWidget(t)
	return t
}

// CreateRenderer 实现 fyne.Widget 接口
func (t *TabBar) CreateRenderer() fyne.WidgetRenderer {
	addBtn := widget.NewButton("+", func() {
		if t.onAdd != nil {
			t.onAdd()
		}
	})

	t.hbox = container.NewHBox()
	t.scroll = container.NewHScroll(t.hbox)

	content := container.NewBorder(nil, nil, nil, addBtn, t.scroll)
	return widget.NewSimpleRenderer(content)
}

// AddTab 添加一个标签
func (t *TabBar) AddTab(item TabItem) {
	t.tabs = append(t.tabs, &item)
	t.rebuild()
	if len(t.tabs) == 1 {
		t.SetActive(0)
	}
}

// RemoveTab 移除指定位置的标签
func (t *TabBar) RemoveTab(index int) {
	if index < 0 || index >= len(t.tabs) {
		return
	}
	t.tabs = append(t.tabs[:index], t.tabs[index+1:]...)
	// 调整活动索引
	if t.active >= len(t.tabs) {
		t.active = len(t.tabs) - 1
	}
	if t.active < 0 {
		t.active = -1
	}
	t.rebuild()
}

// SetActive 设置活动标签
func (t *TabBar) SetActive(index int) {
	if index < 0 || index >= len(t.tabs) {
		return
	}
	t.active = index
	t.rebuild()
	if t.onChange != nil {
		t.onChange(index)
	}
}

// SetOnChange 设置标签切换回调
func (t *TabBar) SetOnChange(fn func(index int)) { t.onChange = fn }

// SetOnAdd 设置添加按钮回调
func (t *TabBar) SetOnAdd(fn func()) { t.onAdd = fn }

// SetOnClose 设置关闭回调
func (t *TabBar) SetOnClose(fn func(index int)) { t.onClose = fn }

// GetTabCount 返回标签数量
func (t *TabBar) GetTabCount() int { return len(t.tabs) }

// Clear 清空所有标签
func (t *TabBar) Clear() {
	t.tabs = make([]*TabItem, 0)
	t.active = -1
	t.rebuild()
}

// rebuild 重建标签按钮
func (t *TabBar) rebuild() {
	if t.hbox == nil {
		return
	}
	t.hbox.RemoveAll()
	for i, item := range t.tabs {
		btn := newTabButton(item, i == t.active, i, func(idx int) {
			t.SetActive(idx)
		}, func(idx int) {
			if t.onClose != nil {
				t.onClose(idx)
			}
		})
		t.hbox.Add(btn)
	}
	t.hbox.Refresh()
	t.scroll.Refresh()
}

// tabButton 是单个标签按钮
type tabButton struct {
	widget.BaseWidget
	item    *TabItem
	active  bool
	index   int
	onClick func(int)
	onClose func(int)
}

func newTabButton(item *TabItem, active bool, index int, onClick func(int), onClose func(int)) *tabButton {
	b := &tabButton{
		item:    item,
		active:  active,
		index:   index,
		onClick: onClick,
		onClose: onClose,
	}
	b.ExtendBaseWidget(b)
	return b
}

// Tapped 实现 fyne.Tappable 接口，点击切换标签
func (b *tabButton) Tapped(*fyne.PointEvent) {
	if b.onClick != nil {
		b.onClick(b.index)
	}
}

func (b *tabButton) CreateRenderer() fyne.WidgetRenderer {
	label := widget.NewLabel(b.item.Title)
	label.Truncation = fyne.TextTruncateClip

	var content fyne.CanvasObject
	if b.item.Closable {
		closeBtn := widget.NewButton("✕", func() {
			if b.onClose != nil {
				b.onClose(b.index)
			}
		})
		content = container.NewHBox(label, closeBtn)
	} else {
		content = container.NewHBox(label)
	}

	bg := canvas.NewRectangle(colorInactive)

	stack := container.NewStack(bg, content)

	return &tabButtonRenderer{
		button: b,
		label:  label,
		bg:     bg,
		stack:  stack,
	}
}

type tabButtonRenderer struct {
	button *tabButton
	label  *widget.Label
	bg     *canvas.Rectangle
	stack  *fyne.Container
}

func (r *tabButtonRenderer) Layout(size fyne.Size) {
	r.stack.Resize(size)
}

func (r *tabButtonRenderer) MinSize() fyne.Size {
	return r.stack.MinSize()
}

func (r *tabButtonRenderer) Refresh() {
	r.label.SetText(r.button.item.Title)
	if r.button.active {
		r.bg.FillColor = colorActive
	} else {
		r.bg.FillColor = colorInactive
	}
	r.bg.Refresh()
}

func (r *tabButtonRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.stack}
}

func (r *tabButtonRenderer) Destroy() {}
