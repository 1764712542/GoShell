package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
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
// 支持添加本地/远程/动态(SOCKS5)隧道，以及移除已有隧道。
type TunnelPanel struct {
	widget.BaseWidget
	tunnels   []TunnelRow
	onAdd     func(typ, local, remote string)
	onRemove  func(index int)
	list      *widget.List
	win       fyne.Window
}

// NewTunnelPanel 创建隧道面板
func NewTunnelPanel() *TunnelPanel {
	p := &TunnelPanel{
		tunnels: make([]TunnelRow, 0),
	}
	p.list = widget.NewList(
		func() int { return len(p.tunnels) },
		func() fyne.CanvasObject {
			statusText := canvas.NewText("状态", colorGray)
			statusText.TextSize = 12
			return container.NewBorder(nil, nil,
				container.NewHBox(
					widget.NewIcon(theme.MailComposeIcon()),
					widget.NewLabel("类型"),
				),
				widget.NewButtonWithIcon("", theme.DeleteIcon(), nil),
				container.NewVBox(
					widget.NewLabel("本地 → 远程"),
					statusText,
				),
			)
		},
		func(i widget.ListItemID, obj fyne.CanvasObject) {
			if i >= len(p.tunnels) {
				return
			}
			row := p.tunnels[i]
			border := obj.(*fyne.Container)

			// 左侧：图标 + 类型
			leftBox := border.Objects[0].(*fyne.Container)
			icon := leftBox.Objects[0].(*widget.Icon)
			typeLabel := leftBox.Objects[1].(*widget.Label)

			typeText := ""
			switch row.Type {
			case "local":
				typeText = "本地转发"
				icon.SetResource(theme.DownloadIcon())
			case "remote":
				typeText = "远程转发"
				icon.SetResource(theme.UploadIcon())
			case "dynamic":
				typeText = "动态(SOCKS5)"
				icon.SetResource(theme.ViewRefreshIcon())
			}

			typeLabel.SetText(typeText)
			typeLabel.TextStyle = fyne.TextStyle{Bold: true}

			// 右侧：移除按钮
			removeBtn := border.Objects[1].(*widget.Button)
			idx := i
			removeBtn.OnTapped = func() {
				if p.onRemove != nil {
					p.onRemove(idx)
				}
			}

			// 中间：地址 + 状态
			centerBox := border.Objects[2].(*fyne.Container)
			addrLabel := centerBox.Objects[0].(*widget.Label)
			statusText := centerBox.Objects[1].(*canvas.Text)

			if row.Type == "dynamic" {
				addrLabel.SetText(fmt.Sprintf("监听: %s", row.LocalAddr))
			} else {
				addrLabel.SetText(fmt.Sprintf("%s → %s", row.LocalAddr, row.RemoteAddr))
			}
			addrLabel.TextStyle = fyne.TextStyle{Monospace: true}

			if row.Active && row.Error == "" {
				statusText.Text = "● 活跃"
				statusText.Color = colorGreen
			} else if row.Error != "" {
				statusText.Text = "● 错误: " + row.Error
				statusText.Color = colorRed
			} else {
				statusText.Text = "○ 已停止"
				statusText.Color = colorGray
			}
			statusText.Refresh()
		},
	)
	p.ExtendBaseWidget(p)
	return p
}

// CreateRenderer 实现 fyne.Widget 接口
func (p *TunnelPanel) CreateRenderer() fyne.WidgetRenderer {
	// 添加隧道按钮 - 绑定点击事件到弹出对话框
	addBtn := widget.NewButtonWithIcon("添加隧道", theme.ContentAddIcon(), func() {
		p.showAddTunnelDialog()
	})
	addBtn.Importance = widget.HighImportance

	// 空状态提示
	emptyLabel := widget.NewLabel("暂无隧道\n点击「添加隧道」创建端口转发")
	emptyLabel.Alignment = fyne.TextAlignCenter

	var content fyne.CanvasObject
	if len(p.tunnels) == 0 {
		content = container.NewCenter(emptyLabel)
	} else {
		content = p.list
	}

	wrapper := container.NewBorder(nil, addBtn, nil, nil, content)
	return widget.NewSimpleRenderer(wrapper)
}

// showAddTunnelDialog 显示添加隧道的对话框
func (p *TunnelPanel) showAddTunnelDialog() {
	if p.win == nil {
		// 尝试获取窗口
		p.win = windowForObject(p)
	}
	if p.win == nil {
		return
	}

	// 隧道类型选择
	typeSelect := widget.NewSelect([]string{"本地转发 (-L)", "远程转发 (-R)", "动态 SOCKS5 (-D)"}, nil)
	typeSelect.SetSelectedIndex(0)

	// 本地地址输入
	localEntry := widget.NewEntry()
	localEntry.SetPlaceHolder("例: 127.0.0.1:8080 或 :8080")

	// 远程地址输入
	remoteEntry := widget.NewEntry()
	remoteEntry.SetPlaceHolder("例: 127.0.0.1:80")

	// 表单内容
	form := container.NewVBox(
		widget.NewLabel("隧道类型:"),
		typeSelect,
		widget.NewLabel("本地地址:"),
		localEntry,
		widget.NewLabel("远程地址:"),
		remoteEntry,
	)

	dialog.ShowCustomConfirm("添加隧道", "添加", "取消", form, func(confirm bool) {
		if !confirm {
			return
		}

		local := strings.TrimSpace(localEntry.Text)
		remote := strings.TrimSpace(remoteEntry.Text)

		if local == "" {
			dialog.ShowError(fmt.Errorf("本地地址不能为空"), p.win)
			return
		}

		// 解析隧道类型
		tunnelType := "local"
		remoteAddr := remote
		switch typeSelect.SelectedIndex() {
		case 0:
			tunnelType = "local"
			if remoteAddr == "" {
				dialog.ShowError(fmt.Errorf("远程地址不能为空"), p.win)
				return
			}
		case 1:
			tunnelType = "remote"
			if remoteAddr == "" {
				dialog.ShowError(fmt.Errorf("远程地址不能为空"), p.win)
				return
			}
		case 2:
			tunnelType = "dynamic"
			remoteAddr = ""
		}

		// 调用添加回调
		if p.onAdd != nil {
			p.onAdd(tunnelType, local, remoteAddr)
		}
	}, p.win)
}

// AddTunnel 添加一行隧道
func (p *TunnelPanel) AddTunnel(row TunnelRow) {
	p.tunnels = append(p.tunnels, row)
	p.Refresh()
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

// RemoveTunnel 移除指定隧道
func (p *TunnelPanel) RemoveTunnel(index int) {
	if index < 0 || index >= len(p.tunnels) {
		return
	}
	p.tunnels = append(p.tunnels[:index], p.tunnels[index+1:]...)
	p.Refresh()
}

// Clear 清空隧道列表
func (p *TunnelPanel) Clear() {
	p.tunnels = make([]TunnelRow, 0)
	p.Refresh()
}

// SetOnAdd 设置添加回调
func (p *TunnelPanel) SetOnAdd(fn func(typ, local, remote string)) { p.onAdd = fn }

// SetOnRemove 设置移除回调
func (p *TunnelPanel) SetOnRemove(fn func(index int)) { p.onRemove = fn }

// SetWindow 设置窗口引用（用于显示对话框）
func (p *TunnelPanel) SetWindow(win fyne.Window) { p.win = win }
