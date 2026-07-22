package terminal

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// SearchBar 是终端内搜索栏 widget。
// 它包含一个搜索输入框、上一个/下一个匹配按钮和关闭按钮，
// 并通过 onSearch 回调将查询和方向通知给 TerminalWidget。
type SearchBar struct {
	widget.BaseWidget
	entry    *widget.Entry
	result   *widget.Label
	onSearch func(query string, direction int) // direction: 1=next, -1=prev, 0=new search
	onClose  func()
}

// NewSearchBar 创建一个搜索栏。
// onSearch 在用户按下回车或点击上/下一个按钮时被调用；
// onClose 在用户点击关闭按钮时被调用。
func NewSearchBar(onSearch func(string, int), onClose func()) *SearchBar {
	s := &SearchBar{
		onSearch: onSearch,
		onClose:  onClose,
	}
	s.entry = widget.NewEntry()
	s.entry.SetPlaceHolder("搜索终端输出...")
	s.entry.OnSubmitted = func(q string) {
		if s.onSearch != nil {
			s.onSearch(q, 1)
		}
	}
	s.result = widget.NewLabel("")
	s.result.TextStyle = fyne.TextStyle{Monospace: true}
	s.ExtendBaseWidget(s)
	return s
}

// CreateRenderer 实现 fyne.Widget 接口。
// 布局：左侧 [上一个][下一个] 按钮 | 中间 [输入框][结果标签] | 右侧 [关闭]。
func (s *SearchBar) CreateRenderer() fyne.WidgetRenderer {
	prevBtn := widget.NewButtonWithIcon("", theme.NavigateBackIcon(), func() {
		if s.onSearch != nil {
			s.onSearch(s.entry.Text, -1)
		}
	})
	nextBtn := widget.NewButtonWithIcon("", theme.NavigateNextIcon(), func() {
		if s.onSearch != nil {
			s.onSearch(s.entry.Text, 1)
		}
	})
	closeBtn := widget.NewButtonWithIcon("", theme.CancelIcon(), func() {
		if s.onClose != nil {
			s.onClose()
		}
	})
	content := container.NewBorder(
		nil, nil,
		container.NewHBox(prevBtn, nextBtn),
		closeBtn,
		container.NewHBox(s.entry, s.result),
	)
	return widget.NewSimpleRenderer(content)
}

// SetResult 更新搜索结果标签文本（如 "1/5"）。
func (s *SearchBar) SetResult(text string) { s.result.SetText(text) }

// Focus 尝试将键盘焦点设置到搜索输入框。
func (s *SearchBar) Focus() {
	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	drv := app.Driver()
	if drv == nil {
		return
	}
	c := drv.CanvasForObject(s.entry)
	if c != nil {
		c.Focus(s.entry)
	}
}

// Entry 返回内部输入框，供外部设置文本或查询内容。
func (s *SearchBar) Entry() *widget.Entry { return s.entry }

// Text 返回当前输入框中的查询文本。
func (s *SearchBar) Text() string { return s.entry.Text }

// SetText 设置输入框中的文本。
func (s *SearchBar) SetText(text string) { s.entry.SetText(text) }
