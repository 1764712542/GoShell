package ui

import (
	"errors"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/zhuyao/meatshell/internal/config"
)

// ShowMasterPasswordDialog 在启动时显示主密码解锁对话框。
// 若主密码保护已启用（磁盘上存在校验令牌）且内存中尚未设置主密码，应调用此函数。
// onSuccess 在用户成功验证主密码后回调。
func ShowMasterPasswordDialog(win fyne.Window, onSuccess func()) {
	passwordEntry := widget.NewPasswordEntry()
	passwordEntry.SetPlaceHolder("请输入主密码")

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "主密码", Widget: passwordEntry},
		},
	}

	dialog.ShowCustomConfirm("解锁 GoShell", "解锁", "退出", form, func(confirmed bool) {
		if !confirmed {
			// 用户选择退出
			win.Close()
			return
		}
		pw := passwordEntry.Text
		if pw == "" {
			dialog.ShowInformation("提示", "请输入主密码", win)
			ShowMasterPasswordDialog(win, onSuccess)
			return
		}
		if !config.VerifyMasterPassword(pw) {
			dialog.ShowError(errors.New("主密码不正确"), win)
			ShowMasterPasswordDialog(win, onSuccess)
			return
		}
		// 验证成功，设置内存中的主密码
		config.SetMasterPassword(pw)
		if onSuccess != nil {
			onSuccess()
		}
	}, win)
}

// ShowSetMasterPasswordDialog 显示设置主密码对话框（首次启用或修改主密码时使用）。
// requireOld 为 true 时需要输入旧密码进行验证。
// onSuccess 在主密码成功设置后回调。
func ShowSetMasterPasswordDialog(win fyne.Window, store *config.Store, requireOld bool, onSuccess func()) {
	var oldEntry *widget.Entry
	newEntry := widget.NewPasswordEntry()
	newEntry.SetPlaceHolder("输入新主密码")
	confirmEntry := widget.NewPasswordEntry()
	confirmEntry.SetPlaceHolder("再次输入新主密码")

	items := []*widget.FormItem{
		{Text: "新密码", Widget: newEntry},
		{Text: "确认密码", Widget: confirmEntry},
	}

	if requireOld {
		oldEntry = widget.NewPasswordEntry()
		oldEntry.SetPlaceHolder("输入当前主密码")
		items = append([]*widget.FormItem{{Text: "旧密码", Widget: oldEntry}}, items...)
	}

	form := &widget.Form{Items: items}

	title := "设置主密码"
	if requireOld {
		title = "修改主密码"
	}

	dialog.ShowCustomConfirm(title, "确定", "取消", form, func(confirmed bool) {
		if !confirmed {
			return
		}

		oldPw := ""
		if requireOld {
			oldPw = oldEntry.Text
		}
		newPw := newEntry.Text
		confirmPw := confirmEntry.Text

		if newPw == "" {
			dialog.ShowInformation("提示", "新主密码不能为空", win)
			return
		}
		if newPw != confirmPw {
			dialog.ShowError(errors.New("两次输入的密码不一致"), win)
			return
		}

		if err := store.ChangeMasterPassword(oldPw, newPw); err != nil {
			dialog.ShowError(err, win)
			return
		}

		dialog.ShowInformation("成功", "主密码已设置", win)
		if onSuccess != nil {
			onSuccess()
		}
	}, win)
}
