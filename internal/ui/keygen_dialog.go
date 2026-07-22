package ui

import (
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/zhuyao/meatshell/internal/ssh"
)

// KeygenDialog 是 SSH 密钥生成对话框。
// 提供密钥类型选择、注释输入、生成按钮、私钥/公钥预览和保存功能。
type KeygenDialog struct {
	win fyne.Window

	// 表单字段
	typeSelect  *widget.Select
	commentEntry *widget.Entry

	// 输出区域
	privateText *widget.Entry
	publicText  *widget.Entry

	// 生成的密钥（缓存）
	privateKey string
	publicKey  string
}

// ShowKeygenDialog 创建并显示 SSH 密钥生成对话框。
func ShowKeygenDialog(win fyne.Window) {
	d := &KeygenDialog{win: win}
	d.build()
}

// build 构建对话框 UI 并显示。
func (d *KeygenDialog) build() {
	// 密钥类型选择
	d.typeSelect = widget.NewSelect(
		[]string{"Ed25519", "RSA 4096", "ECDSA"},
		func(s string) {},
	)
	d.typeSelect.SetSelected("Ed25519")

	// 注释/标签输入
	d.commentEntry = widget.NewEntry()
	d.commentEntry.SetPlaceHolder("user@host（可选）")

	// 私钥输出（多行只读）
	d.privateText = widget.NewMultiLineEntry()
	d.privateText.SetMinRowsVisible(6)
	d.privateText.SetPlaceHolder("点击「生成」按钮创建私钥...")
	d.privateText.Wrapping = fyne.TextWrapOff

	// 公钥输出（多行只读）
	d.publicText = widget.NewMultiLineEntry()
	d.publicText.SetMinRowsVisible(3)
	d.publicText.SetPlaceHolder("点击「生成」按钮创建公钥...")
	d.publicText.Wrapping = fyne.TextWrapOff

	// 按钮区域
	generateBtn := widget.NewButtonWithIcon("生成", theme.ViewRefreshIcon(), d.generate)
	copyPrivateBtn := widget.NewButtonWithIcon("复制私钥", theme.ContentCopyIcon(), func() {
		if d.privateKey != "" {
			d.win.Clipboard().SetContent(d.privateKey)
		}
	})
	copyPublicBtn := widget.NewButtonWithIcon("复制公钥", theme.ContentCopyIcon(), func() {
		if d.publicKey != "" {
			d.win.Clipboard().SetContent(d.publicKey)
		}
	})
	saveBtn := widget.NewButtonWithIcon("保存私钥到文件", theme.DocumentSaveIcon(), d.savePrivateKey)

	// 表单
	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "密钥类型", Widget: d.typeSelect},
			{Text: "注释", Widget: d.commentEntry},
		},
	}

	// 私钥区域
	privateLabel := widget.NewLabel("私钥 (OpenSSH PEM)")
	privateBox := container.NewBorder(
		container.NewHBox(privateLabel, copyPrivateBtn),
		nil, nil, nil,
		d.privateText,
	)

	// 公钥区域
	publicLabel := widget.NewLabel("公钥 (authorized_keys)")
	publicBox := container.NewBorder(
		container.NewHBox(publicLabel, copyPublicBtn),
		nil, nil, nil,
		d.publicText,
	)

	// 整体布局
	content := container.NewVBox(
		form,
		container.NewHBox(generateBtn, saveBtn),
		privateBox,
		publicBox,
	)

	dialog.ShowCustom("SSH 密钥生成", "关闭", content, d.win)
}

// generate 生成密钥对并填充到文本区域。
func (d *KeygenDialog) generate() {
	keyType := d.typeSelect.Selected
	bits := 0

	var algo string
	switch keyType {
	case "Ed25519":
		algo = "ed25519"
	case "RSA 4096":
		algo = "rsa"
		bits = 4096
	case "ECDSA":
		algo = "ecdsa"
	default:
		dialog.ShowError(nil, d.win)
		return
	}

	priv, pub, err := ssh.GenerateKeyPair(algo, bits)
	if err != nil {
		dialog.ShowError(err, d.win)
		return
	}

	// 追加注释到公钥
	comment := d.commentEntry.Text
	pub = ssh.AppendCommentToPublicKey(pub, comment)

	d.privateKey = priv
	d.publicKey = pub
	d.privateText.SetText(priv)
	d.publicText.SetText(pub)
}

// savePrivateKey 将私钥保存到文件。
func (d *KeygenDialog) savePrivateKey() {
	if d.privateKey == "" {
		dialog.ShowInformation("提示", "请先生成密钥", d.win)
		return
	}

	dialog.ShowFileSave(func(writer fyne.URIWriteCloser, err error) {
		if err != nil {
			dialog.ShowError(err, d.win)
			return
		}
		if writer == nil {
			return
		}
		defer writer.Close()

		_, writeErr := writer.Write([]byte(d.privateKey))
		if writeErr != nil {
			dialog.ShowError(writeErr, d.win)
			return
		}

		// 设置文件权限为 0600（仅所有者可读写）
		if path := writer.URI().Path(); path != "" {
			_ = os.Chmod(filepath.Clean(path), 0600)
		}

		dialog.ShowInformation("成功", "私钥已保存", d.win)
	}, d.win)
}
