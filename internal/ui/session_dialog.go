package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
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
		[]string{string(config.SessionSSH), string(config.SessionSerial), string(config.SessionTelnet)},
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
}

// Show 显示对话框
func (d *SessionDialog) Show() {
	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "名称", Widget: d.nameEntry},
			{Text: "类型", Widget: d.typeSelect},
			{Text: "分组", Widget: d.groupEntry},
			{Text: "主机/设备", Widget: d.hostEntry},
			{Text: "端口/波特率", Widget: d.portEntry},
			{Text: "用户名", Widget: d.usernameEntry},
			{Text: "认证方式", Widget: d.authSelect},
			{Text: "密码", Widget: d.passwordEntry},
			{Text: "私钥", Widget: d.privateKeyEntry},
			{Text: "私钥口令", Widget: d.passphraseEntry},
			{Text: "终端类型", Widget: d.termTypeEntry},
			{Text: "字体大小", Widget: d.fontSizeEntry},
		},
		OnSubmit: func() {
			d.save()
		},
		OnCancel: func() {},
	}

	dialog.ShowCustom("会话设置", "保存", form, d.win)
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

	// 解析端口
	if port, err := fmtStrToInt(d.portEntry.Text); err == nil {
		s.Port = port
	}

	// 解析字体大小
	if fs, err := fmtStrToFloat(d.fontSizeEntry.Text); err == nil {
		s.FontSize = fs
	}

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

	list = widget.NewList(
		func() int { return len(sessions) },
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewLabel("名称"),
				widget.NewLabel(""),
				widget.NewButton("连接", nil),
				widget.NewButton("编辑", nil),
				widget.NewButton("删除", nil),
			)
		},
		func(i widget.ListItemID, obj fyne.CanvasObject) {
			if i >= len(sessions) {
				return
			}
			sess := sessions[i]
			hbox := obj.(*fyne.Container)
			nameLabel := hbox.Objects[0].(*widget.Label)
			metaLabel := hbox.Objects[1].(*widget.Label)
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

	dialog.ShowCustom("会话列表", "关闭", list, win)
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
