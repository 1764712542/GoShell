package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// TunnelRow 表示一行隧道信息
type TunnelRow struct {
	Type       string // local / remote / dynamic
	LocalAddr  string
	RemoteAddr string
	Active     bool
	Error      string
}

// TunnelPanel 是 SSH 隧道管理面板，显示隧道列表和状态。
type TunnelPanel struct {
	widget.BaseWidget
	tunnels  []TunnelRow
	onAdd    func(typ, local, remote string)
	onRemove func(index int)
	list     *widget.List
}

// NewTunnelPanel 创建隧道面板
func NewTunnelPanel() *TunnelPanel {
	p := &TunnelPanel{
		tunnels: make([]TunnelRow, 0),
	}
	p.list = widget.NewList(
		func() int { return len(p.tunnels) },
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewLabel(""),
				widget.NewLabel(""),
				widget.NewLabel(""),
				widget.NewButton("移除", nil),
			)
		},
		func(i widget.ListItemID, obj fyne.CanvasObject) {
			if i >= len(p.tunnels) {
				return
			}
			row := p.tunnels[i]
			hbox := obj.(*fyne.Container)
			labels := hbox.Objects
			statusText := "●"
			if row.Active {
				statusText = "● 活跃"
			} else if row.Error != "" {
				statusText = "● 错误"
			}
			labels[0].(*widget.Label).SetText(statusText)
			labels[1].(*widget.Label).SetText(fmt.Sprintf("%s: %s", row.Type, row.LocalAddr))
			if row.RemoteAddr != "" {
				labels[2].(*widget.Label).SetText("→ " + row.RemoteAddr)
			} else {
				labels[2].(*widget.Label).SetText("")
			}
			// 移除按钮
			btn := labels[3].(*widget.Button)
			btn.OnTapped = func() {
				if p.onRemove != nil {
					p.onRemove(i)
				}
			}
		},
	)
	p.ExtendBaseWidget(p)
	return p
}

// CreateRenderer 实现 fyne.Widget 接口
func (p *TunnelPanel) CreateRenderer() fyne.WidgetRenderer {
	addBtn := widget.NewButton("添加隧道", nil)

	content := container.NewBorder(nil, addBtn, nil, nil, p.list)
	return widget.NewSimpleRenderer(content)
}

// AddTunnel 添加一行隧道
func (p *TunnelPanel) AddTunnel(row TunnelRow) {
	p.tunnels = append(p.tunnels, row)
	p.list.Refresh()
}

// UpdateTunnelStatus 更新指定隧道的状态
func (p *TunnelPanel) UpdateTunnelStatus(index int, active bool, errMsg string) {
	if index < 0 || index >= len(p.tunnels) {
		return
	}
	p.tunnels[index].Active = active
	p.tunnels[index].Error = errMsg
	p.list.Refresh()
}

// Clear 清空隧道列表
func (p *TunnelPanel) Clear() {
	p.tunnels = make([]TunnelRow, 0)
	p.list.Refresh()
}

// SetOnAdd 设置添加回调
func (p *TunnelPanel) SetOnAdd(fn func(typ, local, remote string)) { p.onAdd = fn }

// SetOnRemove 设置移除回调
func (p *TunnelPanel) SetOnRemove(fn func(index int)) { p.onRemove = fn }
