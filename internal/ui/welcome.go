package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// WelcomePage 是没有打开标签页时显示的欢迎页。
// 提供新建会话、打开会话列表的入口，以及快捷键提示。
type WelcomePage struct {
	widget.BaseWidget
	onNewSession  func()
	onOpenSession func()
	onAbout       func()
}

// NewWelcomePage 创建欢迎页
func NewWelcomePage() *WelcomePage {
	w := &WelcomePage{}
	w.ExtendBaseWidget(w)
	return w
}

// CreateRenderer 实现 fyne.Widget 接口
func (w *WelcomePage) CreateRenderer() fyne.WidgetRenderer {
	title := widget.NewLabelWithStyle("Meatshell", fyne.TextAlignCenter, fyne.TextStyle{Bold: true, Italic: true})

	subtitle := widget.NewLabelWithStyle("SSH / 串口 / Telnet 终端客户端", fyne.TextAlignCenter, fyne.TextStyle{})

	// 操作按钮
	newBtn := widget.NewButton("新建会话", func() {
		if w.onNewSession != nil {
			w.onNewSession()
		}
	})
	newBtn.Importance = widget.HighImportance

	openBtn := widget.NewButton("打开会话列表", func() {
		if w.onOpenSession != nil {
			w.onOpenSession()
		}
	})

	aboutBtn := widget.NewButton("关于", func() {
		if w.onAbout != nil {
			w.onAbout()
		}
	})

	// 按钮组
	buttons := container.NewVBox(
		newBtn,
		openBtn,
		aboutBtn,
	)

	// 快捷键提示
	hints := widget.NewLabel(
		"快捷键：\n" +
			"  Ctrl+N  新建会话\n" +
			"  Ctrl+O  打开会话列表\n" +
			"  Ctrl+W  关闭当前标签\n" +
			"  Ctrl+Tab  切换标签\n" +
			"  Ctrl+B  广播命令")
	hints.Alignment = fyne.TextAlignLeading

	// 居中布局
	content := container.NewCenter(
		container.NewVBox(
			title,
			subtitle,
			widget.NewSeparator(),
			buttons,
			widget.NewSeparator(),
			hints,
		),
	)
	return widget.NewSimpleRenderer(content)
}

// SetOnNewSession 设置新建会话回调
func (w *WelcomePage) SetOnNewSession(fn func()) { w.onNewSession = fn }

// SetOnOpenSession 设置打开会话列表回调
func (w *WelcomePage) SetOnOpenSession(fn func()) { w.onOpenSession = fn }

// SetOnAbout 设置关于回调
func (w *WelcomePage) SetOnAbout(fn func()) { w.onAbout = fn }

// 确保 WelcomePage 实现了 fyne.Widget 接口（编译期检查）
var _ fyne.Widget = (*WelcomePage)(nil)
