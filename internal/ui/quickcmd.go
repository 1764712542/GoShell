package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// QuickCmdBar 是底部快捷命令栏，支持命令输入、历史记录和广播。
type QuickCmdBar struct {
	widget.BaseWidget
	entry      *widget.Entry
	history    []string
	histIndex  int
	onExecute  func(cmd string)
	onBroadcast func(cmd string)
}

// NewQuickCmdBar 创建快捷命令栏
func NewQuickCmdBar() *QuickCmdBar {
	q := &QuickCmdBar{
		history:   make([]string, 0),
		histIndex: -1,
	}
	q.entry = widget.NewEntry()
	q.entry.SetPlaceHolder("输入命令，Enter 发送到当前会话...")
	q.entry.OnSubmitted = func(cmd string) {
		if cmd == "" {
			return
		}
		q.AddHistory(cmd)
		if q.onExecute != nil {
			q.onExecute(cmd)
		}
		q.entry.SetText("")
	}
	q.ExtendBaseWidget(q)
	return q
}

// CreateRenderer 实现 fyne.Widget 接口
func (q *QuickCmdBar) CreateRenderer() fyne.WidgetRenderer {
	sendBtn := widget.NewButton("发送", func() {
		cmd := q.entry.Text
		if cmd == "" {
			return
		}
		q.AddHistory(cmd)
		if q.onExecute != nil {
			q.onExecute(cmd)
		}
		q.entry.SetText("")
	})

	broadcastBtn := widget.NewButton("广播", func() {
		cmd := q.entry.Text
		if cmd == "" {
			return
		}
		q.AddHistory(cmd)
		if q.onBroadcast != nil {
			q.onBroadcast(cmd)
		}
		q.entry.SetText("")
	})

	content := container.NewBorder(nil, nil, nil,
		container.NewHBox(sendBtn, broadcastBtn),
		q.entry,
	)
	return widget.NewSimpleRenderer(content)
}

// AddHistory 添加命令到历史记录
func (q *QuickCmdBar) AddHistory(cmd string) {
	q.history = append(q.history, cmd)
	q.histIndex = len(q.history)
}

// SetOnExecute 设置发送命令回调
func (q *QuickCmdBar) SetOnExecute(fn func(cmd string)) { q.onExecute = fn }

// SetOnBroadcast 设置广播命令回调
func (q *QuickCmdBar) SetOnBroadcast(fn func(cmd string)) { q.onBroadcast = fn }

// Focus 设置焦点到输入框
func (q *QuickCmdBar) Focus() {
	q.entry.FocusGained()
}
