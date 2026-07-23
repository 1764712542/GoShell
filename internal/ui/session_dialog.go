package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/zhuyao/meatshell/internal/config"
)

// SessionDialog 是会话编辑对话框，用于创建或编辑 SSH/Serial/Telnet 会话。
type SessionDialog struct {
	win     fyne.Window
	session *config.Session

	// 表单字段
	nameEntry       *widget.Entry
	typeSelect      *widget.Select
	hostEntry       *widget.Entry
	portEntry       *widget.Entry
	usernameEntry   *widget.Entry
	passwordEntry   *widget.Entry
	privateKeyEntry *widget.Entry
	passphraseEntry *widget.Entry
	authSelect      *widget.Select
	groupEntry      *widget.Entry
	termTypeEntry   *widget.Entry
	fontSizeEntry   *widget.Entry
	shellEntry      *widget.Entry // 本地终端 shell 路径
	agentForwardingCheck *widget.Check // SSH Agent Forwarding

	// 代理字段
	proxyEnabled  *widget.Check
	proxyType     *widget.Select
	proxyHost     *widget.Entry
	proxyPort     *widget.Entry
	proxyUsername *widget.Entry
	proxyPassword *widget.Entry

	// proxyContainer 在 proxyEnabled 切换时显示/隐藏代理字段
	proxyContainer *fyne.Container

	onSave func(*config.Session)
}

// NewSessionDialog 创建会话编辑对话框。
// session 为 nil 表示新建，否则编辑现有会话。
func NewSessionDialog(session *config.Session, win fyne.Window, onSave func(*config.Session)) *SessionDialog {
	d := &SessionDialog{
		win:    win,
		onSave: onSave,
	}

	if session == nil {
		d.session = config.NewSession("新会话", config.SessionSSH)
	} else {
		// 复制一份，避免直接修改原对象
		clone := *session
		d.session = &clone
	}

	d.buildFields()
	return d
}

// buildFields 构建表单字段
func (d *SessionDialog) buildFields() {
	s := d.session

	d.nameEntry = widget.NewEntry()
	d.nameEntry.SetText(s.Name)

	d.typeSelect = widget.NewSelect(
		[]string{
			string(config.SessionSSH),
			string(config.SessionSerial),
			string(config.SessionTelnet),
			string(config.SessionRLogin),
			string(config.SessionFTP),
			string(config.SessionMosh),
			string(config.SessionLocal),
		},
		func(val string) {
			d.session.Type = config.SessionType(val)
		},
	)
	d.typeSelect.SetSelected(string(s.Type))

	d.hostEntry = widget.NewEntry()
	d.hostEntry.SetText(s.Host)

	d.portEntry = widget.NewEntry()
	if s.Port != 0 {
		d.portEntry.SetText(fmt.Sprintf("%d", s.Port))
	}

	d.usernameEntry = widget.NewEntry()
	d.usernameEntry.SetText(s.Username)

	d.passwordEntry = widget.NewPasswordEntry()
	d.passwordEntry.SetText(s.Password)

	d.privateKeyEntry = widget.NewMultiLineEntry()
	d.privateKeyEntry.SetText(s.PrivateKey)
	d.privateKeyEntry.SetMinRowsVisible(3)

	d.passphraseEntry = widget.NewPasswordEntry()
	d.passphraseEntry.SetText(s.Passphrase)

	d.authSelect = widget.NewSelect(
		[]string{string(config.AuthPassword), string(config.AuthPublicKey), string(config.AuthKeyboard)},
		func(val string) {
			d.session.AuthMethod = config.AuthMethod(val)
		},
	)
	d.authSelect.SetSelected(string(s.AuthMethod))

	d.groupEntry = widget.NewEntry()
	d.groupEntry.SetText(s.Group)

	d.termTypeEntry = widget.NewEntry()
	d.termTypeEntry.SetText(s.TermType)

	d.fontSizeEntry = widget.NewEntry()
	if s.FontSize != 0 {
		d.fontSizeEntry.SetText(fmt.Sprintf("%.1f", s.FontSize))
	}

	// 本地终端 shell 路径（留空则自动检测系统默认 shell）
	d.shellEntry = widget.NewEntry()
	d.shellEntry.SetPlaceHolder("留空则自动检测（如 /bin/bash）")
	d.shellEntry.SetText(s.Shell)

	// SSH Agent Forwarding 复选框
	d.agentForwardingCheck = widget.NewCheck("启用 Agent Forwarding", nil)
	d.agentForwardingCheck.SetChecked(s.AgentForwarding)

	// 代理字段初始化
	d.proxyEnabled = widget.NewCheck("通过代理连接", nil)
	d.proxyType = widget.NewSelect([]string{"socks5", "http"}, nil)
	d.proxyHost = widget.NewEntry()
	d.proxyHost.SetPlaceHolder("代理主机（如 127.0.0.1）")
	d.proxyPort = widget.NewEntry()
	d.proxyPort.SetPlaceHolder("代理端口（如 1080）")
	d.proxyUsername = widget.NewEntry()
	d.proxyUsername.SetPlaceHolder("代理用户名（可选）")
	d.proxyPassword = widget.NewPasswordEntry()
	d.proxyPassword.SetPlaceHolder("代理密码（可选）")

	// 编辑现有会话时填充代理字段
	if s.Proxy != nil {
		d.proxyEnabled.SetChecked(true)
		d.proxyType.SetSelected(s.Proxy.Type)
		d.proxyHost.SetText(s.Proxy.Host)
		if s.Proxy.Port != 0 {
			d.proxyPort.SetText(fmt.Sprintf("%d", s.Proxy.Port))
		}
		d.proxyUsername.SetText(s.Proxy.Username)
		d.proxyPassword.SetText(s.Proxy.Password)
	} else {
		d.proxyEnabled.SetChecked(false)
		d.proxyType.SetSelected("socks5")
	}

	// 代理字段容器：在 proxyEnabled 切换时显示/隐藏
	d.proxyContainer = container.NewVBox(
		&widget.Form{Items: []*widget.FormItem{
			{Text: "代理类型", Widget: d.proxyType},
			{Text: "代理主机", Widget: d.proxyHost},
			{Text: "代理端口", Widget: d.proxyPort},
			{Text: "代理用户名", Widget: d.proxyUsername},
			{Text: "代理密码", Widget: d.proxyPassword},
		}},
	)
	d.updateProxyVisibility()

	d.proxyEnabled.OnChanged = func(checked bool) {
		d.updateProxyVisibility()
	}
}

// updateProxyVisibility 根据代理开关状态显示或隐藏代理字段
func (d *SessionDialog) updateProxyVisibility() {
	if d.proxyEnabled.Checked {
		d.proxyContainer.Show()
	} else {
		d.proxyContainer.Hide()
	}
}

// Show 显示对话框
func (d *SessionDialog) Show() {
	// 根据会话类型动态构建表单字段
	items := []*widget.FormItem{
		{Text: "名称", Widget: d.nameEntry},
		{Text: "类型", Widget: d.typeSelect},
		{Text: "分组", Widget: d.groupEntry},
	}

	// 本地终端：显示 shell 选择字段，不需要 host/port/auth
	if d.session.Type == config.SessionLocal {
		items = append(items,
			&widget.FormItem{Text: "Shell 路径", Widget: d.shellEntry},
		)
	} else {
		// 其他类型需要主机和端口
		items = append(items,
			&widget.FormItem{Text: "主机/设备", Widget: d.hostEntry},
			&widget.FormItem{Text: "端口/波特率", Widget: d.portEntry},
		)

		// SSH/Telnet/RLogin/FTP/Mosh 需要用户名
		if d.session.Type == config.SessionSSH || d.session.Type == config.SessionTelnet ||
			d.session.Type == config.SessionRLogin || d.session.Type == config.SessionFTP ||
			d.session.Type == config.SessionMosh {
			items = append(items,
				&widget.FormItem{Text: "用户名", Widget: d.usernameEntry},
			)
		}

		// SSH 需要认证方式和密钥
		if d.session.Type == config.SessionSSH {
			items = append(items,
				&widget.FormItem{Text: "认证方式", Widget: d.authSelect},
				&widget.FormItem{Text: "密码", Widget: d.passwordEntry},
				&widget.FormItem{Text: "私钥", Widget: d.privateKeyEntry},
				&widget.FormItem{Text: "私钥口令", Widget: d.passphraseEntry},
				&widget.FormItem{Text: "Agent 转发", Widget: d.agentForwardingCheck},
			)
		}

		// FTP 需要密码
		if d.session.Type == config.SessionFTP {
			items = append(items,
				&widget.FormItem{Text: "密码", Widget: d.passwordEntry},
			)
		}

		// Mosh 使用 SSH 认证
		if d.session.Type == config.SessionMosh {
			items = append(items,
				&widget.FormItem{Text: "密码", Widget: d.passwordEntry},
			)
		}

		// 非本地终端类型显示代理设置
		items = append(items,
			&widget.FormItem{Text: "代理", Widget: d.proxyEnabled},
		)
	}

	// 终端设置（所有类型通用）
	items = append(items,
		&widget.FormItem{Text: "终端类型", Widget: d.termTypeEntry},
		&widget.FormItem{Text: "字体大小", Widget: d.fontSizeEntry},
	)

	// 不设置 Form 的 OnSubmit/OnCancel，避免 Form 渲染自己的按钮。
	// 保存/取消由 ShowCustomConfirm 的对话框按钮处理：
	//   - "保存" → confirmed=true → 调用 save()
	//   - "取消" → confirmed=false → 对话框自动关闭
	form := &widget.Form{
		Items: items,
	}

	// 主表单 + 代理字段容器（代理容器在表单下方，根据开关显示/隐藏）
	content := container.NewVBox(form, d.proxyContainer)

	dialog.ShowCustomConfirm("会话设置", "保存", "取消", content, func(confirmed bool) {
		if confirmed {
			d.save()
		}
	}, d.win)
}

// save 收集表单数据并调用回调
func (d *SessionDialog) save() {
	s := d.session
	s.Name = d.nameEntry.Text
	s.Host = d.hostEntry.Text
	s.Username = d.usernameEntry.Text
	s.Password = d.passwordEntry.Text
	s.PrivateKey = d.privateKeyEntry.Text
	s.Passphrase = d.passphraseEntry.Text
	s.Group = d.groupEntry.Text
	s.TermType = d.termTypeEntry.Text
	s.Shell = d.shellEntry.Text

	// 解析端口
	if port, err := fmtStrToInt(d.portEntry.Text); err == nil {
		s.Port = port
	}

	// 解析字体大小
	if fs, err := fmtStrToFloat(d.fontSizeEntry.Text); err == nil {
		s.FontSize = fs
	}

	// 处理代理配置
	if d.proxyEnabled.Checked {
		proxyPort := 0
		if port, err := fmtStrToInt(d.proxyPort.Text); err == nil {
			proxyPort = port
		}
		s.Proxy = &config.ProxyConfig{
			Type:     d.proxyType.Selected,
			Host:     d.proxyHost.Text,
			Port:     proxyPort,
			Username: d.proxyUsername.Text,
			Password: d.proxyPassword.Text,
		}
	} else {
		s.Proxy = nil
	}

	// 保存 Agent Forwarding 设置
	s.AgentForwarding = d.agentForwardingCheck.Checked

	// 认证方式已通过 select 回调设置
	// 类型已通过 select 回调设置

	if err := s.Validate(); err != nil {
		dialog.ShowError(err, d.win)
		return
	}

	if d.onSave != nil {
		d.onSave(s)
	}
}

// ShowSessionListDialog 显示会话列表对话框，供用户选择要连接的会话
func ShowSessionListDialog(store *config.Store, win fyne.Window, onConnect func(*config.Session)) {
	sessions := store.List()
	if len(sessions) == 0 {
		dialog.ShowInformation("提示", "暂无保存的会话，请先创建新会话", win)
		return
	}

	// 预声明 list 变量，以便在 updateItem 闭包中引用
	var list *widget.List
	// 保存对话框引用，用于点击连接后关闭对话框
	var dlg dialog.Dialog

	list = widget.NewList(
		func() int { return len(sessions) },
		func() fyne.CanvasObject {
			// 名称行（加粗）
			nameLabel := widget.NewLabel("名称")
			nameLabel.TextStyle = fyne.TextStyle{Bold: true}
			// 元信息行（小字）
			metaLabel := widget.NewLabel("")
			metaLabel.TextStyle = fyne.TextStyle{Monospace: true}
			// 名称 + 元信息垂直排列
			infoBox := container.NewVBox(nameLabel, metaLabel)
			// 图标按钮，节省空间
			connectBtn := widget.NewButtonWithIcon("", theme.DownloadIcon(), nil)
			editBtn := widget.NewButtonWithIcon("", theme.DocumentCreateIcon(), nil)
			deleteBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), nil)
			return container.NewHBox(infoBox, layout.NewSpacer(), connectBtn, editBtn, deleteBtn)
		},
		func(i widget.ListItemID, obj fyne.CanvasObject) {
			if i >= len(sessions) {
				return
			}
			sess := sessions[i]
			hbox := obj.(*fyne.Container)
			infoBox := hbox.Objects[0].(*fyne.Container)
			nameLabel := infoBox.Objects[0].(*widget.Label)
			metaLabel := infoBox.Objects[1].(*widget.Label)
			// 图标按钮在 Spacer 之后
			connectBtn := hbox.Objects[2].(*widget.Button)
			editBtn := hbox.Objects[3].(*widget.Button)
			deleteBtn := hbox.Objects[4].(*widget.Button)

			nameLabel.SetText(sess.Name)
			meta := string(sess.Type)
			if sess.Host != "" {
				meta += "  " + sess.Host
				if sess.Port > 0 {
					meta += ":" + fmt.Sprintf("%d", sess.Port)
				}
			}
			if sess.Group != "" {
				meta = "[" + sess.Group + "] " + meta
			}
			metaLabel.SetText(meta)

			connectBtn.OnTapped = func() {
				// 先关闭对话框，避免遮挡新标签页的错误提示
				if dlg != nil {
					dlg.Hide()
				}
				if onConnect != nil {
					onConnect(sess)
				}
			}
			editBtn.OnTapped = func() {
				ed := NewSessionDialog(sess, win, func(updated *config.Session) {
					if err := store.Update(updated); err != nil {
						dialog.ShowError(err, win)
						return
					}
					sessions = store.List()
					list.Refresh()
				})
				ed.Show()
			}
			deleteBtn.OnTapped = func() {
				dialog.ShowConfirm("确认删除", "确定要删除会话 "+sess.Name+" 吗？", func(ok bool) {
					if !ok {
						return
					}
					if err := store.Delete(sess.ID); err != nil {
						dialog.ShowError(err, win)
						return
					}
					sessions = store.List()
					list.Refresh()
				}, win)
			}
		},
	)

	// 使用 NewCustom 获取对话框引用，以便点击连接后关闭
	dlg = dialog.NewCustom("会话列表", "关闭", list, win)
	dlg.Show()
}

// ShowNewSessionDialog 显示新建会话对话框
func ShowNewSessionDialog(store *config.Store, win fyne.Window, onConnect func(*config.Session)) {
	d := NewSessionDialog(nil, win, func(sess *config.Session) {
		if err := store.Add(sess); err != nil {
			dialog.ShowError(err, win)
			return
		}
		if onConnect != nil {
			onConnect(sess)
		}
	})
	d.Show()
}

// fmtStrToInt 将字符串解析为整数
func fmtStrToInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

// fmtStrToFloat 将字符串解析为 float32
func fmtStrToFloat(s string) (float32, error) {
	var f float32
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}
