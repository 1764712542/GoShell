package ui

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// 标签栏颜色（macOS 暗色模式调色板）
var (
	colorActive    = color.RGBA{R: 0x0a, G: 0x84, B: 0xff, A: 0x33} // macOS 蓝低透明度
	colorInactive  = color.RGBA{R: 0x24, G: 0x24, B: 0x34, A: 0xff}
	colorUnderline = color.RGBA{R: 0x0a, G: 0x84, B: 0xff, A: 0xff} // macOS 蓝
	colorTabHover  = color.RGBA{R: 0x35, G: 0x35, B: 0x45, A: 0x80}
	colorToolbarBg = color.RGBA{R: 0x18, G: 0x18, B: 0x25, A: 0xff}
	// 通用状态颜色
	colorGreen = color.RGBA{R: 0x30, G: 0xd1, B: 0x58, A: 0xff} // macOS 绿
	colorRed   = color.RGBA{R: 0xff, G: 0x45, B: 0x3a, A: 0xff} // macOS 红
	colorGray  = color.RGBA{R: 0x8e, G: 0x8e, B: 0x93, A: 0xff}
)

// 标签颜色标记预设（用于快速颜色选择）
var tabColorPresets = []struct {
	Name string
	Hex  string
	RGB  color.RGBA
}{
	{"无", "", color.RGBA{R: 0, G: 0, B: 0, A: 0}},
	{"红", "#f38ba8", color.RGBA{R: 0xf3, G: 0x8b, B: 0xa8, A: 0xff}},
	{"绿", "#a6e3a1", color.RGBA{R: 0xa6, G: 0xe3, B: 0xa1, A: 0xff}},
	{"黄", "#f9e2af", color.RGBA{R: 0xf9, G: 0xe2, B: 0xaf, A: 0xff}},
	{"蓝", "#89b4fa", color.RGBA{R: 0x89, G: 0xb4, B: 0xfa, A: 0xff}},
	{"紫", "#cba6f7", color.RGBA{R: 0xcb, G: 0xa6, B: 0xf7, A: 0xff}},
	{"青", "#94e2d5", color.RGBA{R: 0x94, G: 0xe2, B: 0xd5, A: 0xff}},
	{"橙", "#fab387", color.RGBA{R: 0xfa, G: 0xb3, B: 0x87, A: 0xff}},
}

// tabPadding 是标签按钮的内边距
const tabPadding = 8

// TabItem 表示标签栏中的一个标签项
type TabItem struct {
	ID       string
	Title    string
	Closable bool
	Color    string // 颜色标记（十六进制色值，空字符串表示无标记）
}

// TabBar 是可水平滚动的标签栏组件，支持添加、关闭、切换标签，
// 以及右键菜单、颜色标记和拖拽排序。
type TabBar struct {
	widget.BaseWidget
	tabs     []*TabItem
	active   int
	onChange func(index int)
	onAdd    func()
	onClose  func(index int)

	// 标签页操作回调
	onDuplicate  func(index int)             // 复制标签页
	onRename     func(index int, name string) // 重命名标签页
	onSetColor   func(index int, color string) // 设置颜色标记
	onCloseOthers func(index int)            // 关闭其他
	onCloseRight  func(index int)            // 关闭右侧所有
	onReorder    func(from, to int)         // 拖拽重排序

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
	addBtn := widget.NewButtonWithIcon("", theme.ContentAddIcon(), func() {
		if t.onAdd != nil {
			t.onAdd()
		}
	})

	t.hbox = container.NewHBox()
	t.scroll = container.NewHScroll(t.hbox)

	// 标签栏顶部背景
	bg := canvas.NewRectangle(colorToolbarBg)
	stack := container.NewStack(bg, t.scroll)

	content := container.NewBorder(nil, nil, nil, addBtn, stack)
	return widget.NewSimpleRenderer(content)
}

// AddTab 添加一个标签
func (t *TabBar) AddTab(item TabItem) {
	t.tabs = append(t.tabs, &item)
	t.rebuild()
	if len(t.tabs) == 1 {
		t.SetActiveSilent(0)
	}
}

// SetTabs 批量设置所有标签，避免逐个 AddTab 导致的 O(n²) 重建。
// activeID 指定需要激活的标签 ID，为空则不激活任何标签。
func (t *TabBar) SetTabs(items []TabItem, activeID string) {
	t.tabs = make([]*TabItem, len(items))
	for i, item := range items {
		t.tabs[i] = &item
	}
	t.active = -1
	for i, tab := range t.tabs {
		if tab.ID == activeID {
			t.active = i
			break
		}
	}
	t.rebuild()
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

// SetActive 设置活动标签并触发切换回调
func (t *TabBar) SetActive(index int) {
	if index < 0 || index >= len(t.tabs) {
		return
	}
	if t.active == index {
		return
	}
	t.active = index
	t.refreshButtons()
	if t.onChange != nil {
		t.onChange(index)
	}
}

// SetActiveSilent 设置活动标签但不触发 onChange 回调（用于程序化同步）
func (t *TabBar) SetActiveSilent(index int) {
	if index < 0 || index >= len(t.tabs) {
		return
	}
	t.active = index
	t.refreshButtons()
}

// SetOnChange 设置标签切换回调
func (t *TabBar) SetOnChange(fn func(index int)) { t.onChange = fn }

// SetOnAdd 设置添加按钮回调
func (t *TabBar) SetOnAdd(fn func()) { t.onAdd = fn }

// SetOnClose 设置关闭回调
func (t *TabBar) SetOnClose(fn func(index int)) { t.onClose = fn }

// SetOnDuplicate 设置复制标签页回调
func (t *TabBar) SetOnDuplicate(fn func(index int)) { t.onDuplicate = fn }

// SetOnRename 设置重命名标签页回调
func (t *TabBar) SetOnRename(fn func(index int, name string)) { t.onRename = fn }

// SetOnSetColor 设置颜色标记回调
func (t *TabBar) SetOnSetColor(fn func(index int, color string)) { t.onSetColor = fn }

// SetOnCloseOthers 设置关闭其他标签页回调
func (t *TabBar) SetOnCloseOthers(fn func(index int)) { t.onCloseOthers = fn }

// SetOnCloseRight 设置关闭右侧标签页回调
func (t *TabBar) SetOnCloseRight(fn func(index int)) { t.onCloseRight = fn }

// SetOnReorder 设置拖拽重排序回调
func (t *TabBar) SetOnReorder(fn func(from, to int)) { t.onReorder = fn }

// GetTabCount 返回标签数量
func (t *TabBar) GetTabCount() int { return len(t.tabs) }

// GetTab 返回指定位置的标签
func (t *TabBar) GetTab(index int) *TabItem {
	if index < 0 || index >= len(t.tabs) {
		return nil
	}
	return t.tabs[index]
}

// UpdateTabColor 更新标签颜色并刷新
func (t *TabBar) UpdateTabColor(index int, color string) {
	if index < 0 || index >= len(t.tabs) {
		return
	}
	t.tabs[index].Color = color
	t.refreshButtons()
}

// UpdateTabTitle 更新标签标题并刷新
func (t *TabBar) UpdateTabTitle(index int, title string) {
	if index < 0 || index >= len(t.tabs) {
		return
	}
	t.tabs[index].Title = title
	t.refreshButtons()
}

// MoveTab 移动标签从 from 到 to 位置
func (t *TabBar) MoveTab(from, to int) {
	if from < 0 || from >= len(t.tabs) || to < 0 || to >= len(t.tabs) || from == to {
		return
	}
	tab := t.tabs[from]
	t.tabs = append(t.tabs[:from], t.tabs[from+1:]...)
	t.tabs = append(t.tabs[:to], append([]*TabItem{tab}, t.tabs[to:]...)...)
	// 调整 active
	if t.active == from {
		t.active = to
	} else if from < t.active && to >= t.active {
		t.active--
	} else if from > t.active && to <= t.active {
		t.active++
	}
	t.rebuild()
}

// Clear 清空所有标签
func (t *TabBar) Clear() {
	t.tabs = make([]*TabItem, 0)
	t.active = -1
	t.rebuild()
}

// rebuild 重建标签按钮（仅在增删标签时调用）
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
		}, t)
		t.hbox.Add(btn)
	}
	t.hbox.Refresh()
	t.scroll.Refresh()
}

// refreshButtons 刷新现有按钮的视觉状态（不重建按钮，用于切换 active）
func (t *TabBar) refreshButtons() {
	if t.hbox == nil {
		return
	}
	for i, obj := range t.hbox.Objects {
		btn, ok := obj.(*tabButton)
		if !ok {
			continue
		}
		btn.setActive(i == t.active)
	}
}

// showContextMenu 显示标签页右键菜单
func (t *TabBar) showContextMenu(index int, anchor fyne.CanvasObject) {
	if index < 0 || index >= len(t.tabs) {
		return
	}
	c := fyne.CurrentApp().Driver().CanvasForObject(anchor)
	if c == nil {
		return
	}

	item := t.tabs[index]
	menuItems := []*fyne.MenuItem{
		fyne.NewMenuItem("关闭", func() {
			if t.onClose != nil {
				t.onClose(index)
			}
		}),
		fyne.NewMenuItem("关闭其他标签页", func() {
			if t.onCloseOthers != nil {
				t.onCloseOthers(index)
			}
		}),
		fyne.NewMenuItem("关闭右侧所有标签页", func() {
			if t.onCloseRight != nil {
				t.onCloseRight(index)
			}
		}),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("复制标签页", func() {
			if t.onDuplicate != nil {
				t.onDuplicate(index)
			}
		}),
		fyne.NewMenuItem("重命名...", func() {
			t.promptRename(index)
		}),
	}

	// 颜色标记子菜单
	colorItems := make([]*fyne.MenuItem, 0, len(tabColorPresets))
	for _, preset := range tabColorPresets {
		preset := preset
		label := preset.Name
		if item.Color == preset.Hex {
			label = "✓ " + label
		}
		colorItems = append(colorItems, fyne.NewMenuItem(label, func() {
			if t.onSetColor != nil {
				t.onSetColor(index, preset.Hex)
			}
		}))
	}
	colorMenu := fyne.NewMenu("设置颜色", colorItems...)
	menuItems = append(menuItems, &fyne.MenuItem{Label: "设置颜色", ChildMenu: colorMenu})

	menu := fyne.NewMenu("标签页", menuItems...)
	widget.ShowPopUpMenuAtRelativePosition(menu, c, fyne.NewPos(0, anchor.Size().Height), anchor)
}

// promptRename 弹出重命名对话框
func (t *TabBar) promptRename(index int) {
	if index < 0 || index >= len(t.tabs) {
		return
	}
	win := windowForObject(t)
	if win == nil {
		return
	}
	dialog.ShowEntryDialog("重命名标签页", "请输入新的标签页名称：", func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if t.onRename != nil {
			t.onRename(index, name)
		}
	}, win)
}

// tabMinWidth 是标签按钮的最小宽度（像素），确保标签不会过窄
const tabMinWidth = 120

// tabButton 是单个标签按钮，支持点击切换、右键菜单和拖拽重排序
type tabButton struct {
	widget.BaseWidget
	item    *TabItem
	active  bool
	hovered bool
	index   int
	onClick func(int)
	onClose func(int)
	tabBar  *TabBar

	// 渲染时创建的子组件引用（用于 Refresh 更新）
	closeBtn   *widget.Button
	colorBar   *canvas.Rectangle // 左侧颜色标记条
	dragStartX float32            // 拖拽起始 X 坐标
	dragging   bool
}

func newTabButton(item *TabItem, active bool, index int, onClick func(int), onClose func(int), tabBar *TabBar) *tabButton {
	b := &tabButton{
		item:    item,
		active:  active,
		index:   index,
		onClick: onClick,
		onClose: onClose,
		tabBar:  tabBar,
	}
	b.ExtendBaseWidget(b)
	return b
}

// setActive 更新激活状态并刷新视觉
func (b *tabButton) setActive(active bool) {
	if b.active == active {
		return
	}
	b.active = active
	b.Refresh()
}

// Tapped 实现 fyne.Tappable 接口，点击切换标签
func (b *tabButton) Tapped(*fyne.PointEvent) {
	if b.onClick != nil {
		b.onClick(b.index)
	}
}

// TappedSecondary 实现右键菜单（fyne.Tappable 接口的扩展）
func (b *tabButton) TappedSecondary(*fyne.PointEvent) {
	if b.tabBar != nil {
		b.tabBar.showContextMenu(b.index, b)
	}
}

// MouseIn 实现 desktop.Hoverable 接口
func (b *tabButton) MouseIn(*desktop.MouseEvent) {
	b.hovered = true
	b.Refresh()
}

// MouseOut 实现 fyne.Hoverable 接口
func (b *tabButton) MouseOut() {
	b.hovered = false
	b.Refresh()
}

// Dragged 实现 fyne.Draggable 接口，支持拖拽重排序
func (b *tabButton) Dragged(e *fyne.DragEvent) {
	if b.tabBar == nil {
		return
	}
	// 仅在水平拖拽超过一定距离时触发
	if !b.dragging && absFloat(e.Dragged.DX) > 20 {
		b.dragging = true
	}
}

// DragEnd 实现 fyne.Draggable 接口
func (b *tabButton) DragEnd() {
	if !b.dragging || b.tabBar == nil {
		b.dragging = false
		return
	}
	b.dragging = false

	// 计算拖拽目标位置：基于当前 hbox 中按钮的位置
	if b.tabBar.hbox == nil {
		return
	}
	// 获取当前按钮在 hbox 中的位置，找到最近的目标位置
	pos := b.Position()
	buttonWidth := b.Size().Width
	if buttonWidth <= 0 {
		return
	}

	// 遍历 hbox 中的其他按钮，找到最近的
	bestTarget := b.index
	bestDist := float32(999999)
	for _, obj := range b.tabBar.hbox.Objects {
		btn, ok := obj.(*tabButton)
		if !ok || btn.index == b.index {
			continue
		}
		btnPos := btn.Position()
		btnWidth := btn.Size().Width
		if btnWidth <= 0 {
			continue
		}
		// 计算按钮中心点距离
		btnCenter := btnPos.X + btnWidth/2
		myCenter := pos.X + buttonWidth/2
		dist := absFloat(btnCenter - myCenter)
		if dist < bestDist {
			bestDist = dist
			bestTarget = btn.index
		}
	}

	if bestTarget != b.index && b.tabBar.onReorder != nil {
		b.tabBar.onReorder(b.index, bestTarget)
	}
}

// absFloat 返回绝对值
func absFloat(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// CreateRenderer 实现 fyne.Widget 接口
func (b *tabButton) CreateRenderer() fyne.WidgetRenderer {
	label := widget.NewLabel(b.item.Title)
	label.Truncation = fyne.TextTruncateClip

	// 内容区域：标签文本 + 关闭按钮
	var content fyne.CanvasObject
	if b.item.Closable {
		// 用小号 ✕ 文本作为关闭按钮，避免标准 Button 的 padding 过大
		closeIcon := canvas.NewText("✕", colorGray)
		closeIcon.TextSize = 12
		closeIcon.Hide()
		b.closeBtn = nil // 不再使用 widget.Button

		// 用 BaseWidget 实现点击区域
		closeTap := &closeIconWidget{
			text: closeIcon,
			onTap: func() {
				if b.onClose != nil {
					b.onClose(b.index)
				}
			},
		}
		closeTap.ExtendBaseWidget(closeTap)

		if b.active {
			closeIcon.Show()
		}
		content = container.NewHBox(label, layout.NewSpacer(), closeTap)
	} else {
		content = container.NewHBox(label, layout.NewSpacer())
	}

	// 内边距包装
	padded := container.NewPadded(content)

	// 左侧颜色标记条（3px 宽）
	b.colorBar = canvas.NewRectangle(color.RGBA{})
	b.colorBar.SetMinSize(fyne.NewSize(3, 0))
	if b.item.Color != "" {
		b.colorBar.FillColor = hexColor(b.item.Color)
	} else {
		b.colorBar.FillColor = color.RGBA{R: 0, G: 0, B: 0, A: 0} // 透明
	}

	bg := canvas.NewRectangle(colorInactive)
	bg.CornerRadius = 6 // macOS 风格圆角

	// active 标签的底部强调线
	underline := canvas.NewRectangle(colorUnderline)
	underline.Hide()
	if b.active {
		underline.Show()
	}

	// 左侧颜色条 + 主内容
	withColor := container.NewBorder(nil, nil, b.colorBar, nil, padded)
	// 底部强调线叠在内容下方
	bottom := container.NewBorder(nil, underline, nil, nil, withColor)
	stack := container.NewStack(bg, bottom)

	return &tabButtonRenderer{
		button:    b,
		label:     label,
		bg:        bg,
		underline: underline,
		closeText: getCloseText(content),
		colorBar:  b.colorBar,
		stack:     stack,
	}
}

// closeIconWidget 是一个可点击的小号 ✕ 图标，替代标准 Button 以减小尺寸
type closeIconWidget struct {
	widget.BaseWidget
	text  *canvas.Text
	onTap func()
}

func (c *closeIconWidget) Tapped(*fyne.PointEvent) {
	if c.onTap != nil {
		c.onTap()
	}
}

func (c *closeIconWidget) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(c.text)
}

// getCloseText 从 content 中提取 closeIcon 引用（可能为 nil）
// 布局是 HBox(label, spacer, closeTap)，closeTap 在 Objects[2]
func getCloseText(content fyne.CanvasObject) *canvas.Text {
	hbox, ok := content.(*fyne.Container)
	if !ok || len(hbox.Objects) < 3 {
		return nil
	}
	if ci, ok := hbox.Objects[2].(*closeIconWidget); ok {
		return ci.text
	}
	return nil
}

type tabButtonRenderer struct {
	button    *tabButton
	label     *widget.Label
	bg        *canvas.Rectangle
	underline *canvas.Rectangle
	closeText *canvas.Text
	colorBar  *canvas.Rectangle
	stack     *fyne.Container
}

func (r *tabButtonRenderer) Layout(size fyne.Size) {
	r.stack.Resize(size)
	// 底部强调线高度 2px
	if r.underline.Visible() {
		r.underline.Resize(fyne.NewSize(size.Width, 2))
		r.underline.Move(fyne.NewPos(0, size.Height-2))
	}
	// 左侧颜色条占满高度
	if r.colorBar != nil {
		r.colorBar.Resize(fyne.NewSize(3, size.Height))
	}
}

func (r *tabButtonRenderer) MinSize() fyne.Size {
	min := r.stack.MinSize()
	if min.Width < tabMinWidth {
		min.Width = tabMinWidth
	}
	return min
}

func (r *tabButtonRenderer) Refresh() {
	r.label.SetText(r.button.item.Title)
	if r.button.active {
		r.bg.FillColor = colorActive
		r.underline.Show()
		if r.closeText != nil {
			r.closeText.Show()
		}
	} else if r.button.hovered {
		r.bg.FillColor = colorTabHover
		r.underline.Hide()
		if r.closeText != nil {
			r.closeText.Show()
		}
	} else {
		r.bg.FillColor = colorInactive
		r.underline.Hide()
		if r.closeText != nil {
			r.closeText.Hide()
		}
	}

	// 更新颜色标记条
	if r.colorBar != nil {
		if r.button.item.Color != "" {
			r.colorBar.FillColor = hexColor(r.button.item.Color)
		} else {
			r.colorBar.FillColor = color.RGBA{R: 0, G: 0, B: 0, A: 0}
		}
		r.colorBar.Refresh()
	}

	r.bg.Refresh()
	r.underline.Refresh()
	if r.closeText != nil {
		r.closeText.Refresh()
	}
}

func (r *tabButtonRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.stack}
}

func (r *tabButtonRenderer) Destroy() {}
