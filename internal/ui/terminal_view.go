package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/zhuyao/meatshell/internal/app"
	"github.com/zhuyao/meatshell/internal/sftp"
)

// TerminalView 是单个标签页的完整视图容器，组合终端组件、SFTP 面板和隧道面板。
// 通过顶部工具栏切换显示模式：仅终端 / 终端+SFTP / 终端+隧道。
type TerminalView struct {
	widget.BaseWidget
	tab       *app.Tab
	win       fyne.Window
	sftpView  *SFTPView
	tunnelPnl *TunnelPanel
	termBox   *fyne.Container // 终端组件的容器
	content   *fyne.Container // 可切换的内容区域
	sftpBtn   *widget.Button
	tunnelBtn *widget.Button
	mode      ViewMode
}

// ViewMode 表示终端视图的显示模式
type ViewMode int

const (
	ViewModeTerminal ViewMode = iota // 仅终端
	ViewModeSFTP                     // 终端 + SFTP
	ViewModeTunnel                   // 终端 + 隧道
)

// NewTerminalView 创建终端视图容器。
// tab 必须已经调用 Start()。win 用于显示对话框。
func NewTerminalView(tab *app.Tab, win fyne.Window) *TerminalView {
	v := &TerminalView{
		tab:  tab,
		win:  win,
		mode: ViewModeTerminal,
	}

	// 终端组件容器（包一层以便后续替换/调整）
	if tab.TermWidget() != nil {
		v.termBox = container.NewStack(tab.TermWidget())
	} else {
		v.termBox = container.NewStack(widget.NewLabel("（无终端组件）"))
	}

	// SFTP 面板（延迟到连接成功后初始化 browser）
	v.sftpView = NewSFTPView(nil, win)

	// 隧道面板
	v.tunnelPnl = NewTunnelPanel()

	// 内容区域，默认显示终端
	v.content = container.NewMax(v.termBox)

	// 模式切换按钮
	v.sftpBtn = widget.NewButton("SFTP", func() {
		v.toggleMode(ViewModeSFTP)
	})
	v.tunnelBtn = widget.NewButton("隧道", func() {
		v.toggleMode(ViewModeTunnel)
	})

	v.ExtendBaseWidget(v)
	return v
}

// CreateRenderer 实现 fyne.Widget 接口
func (v *TerminalView) CreateRenderer() fyne.WidgetRenderer {
	// 顶部工具栏
	toolbar := container.NewHBox(
		v.sftpBtn,
		v.tunnelBtn,
		widget.NewButton("仅终端", func() {
			v.setMode(ViewModeTerminal)
		}),
	)

	content := container.NewBorder(toolbar, nil, nil, nil, v.content)
	return widget.NewSimpleRenderer(content)
}

// toggleMode 切换到指定模式，若已是该模式则回到仅终端模式
func (v *TerminalView) toggleMode(mode ViewMode) {
	if v.mode == mode {
		v.setMode(ViewModeTerminal)
		return
	}
	v.setMode(mode)
}

// setMode 设置显示模式并重建内容区域
func (v *TerminalView) setMode(mode ViewMode) {
	v.mode = mode

	// 清空内容区域
	v.content.RemoveAll()

	switch mode {
	case ViewModeTerminal:
		v.content.Add(v.termBox)
		v.sftpBtn.SetText("SFTP")
		v.tunnelBtn.SetText("隧道")
	case ViewModeSFTP:
		// 左右分割：终端 | SFTP
		split := container.NewHSplit(v.termBox, v.sftpView)
		split.SetOffset(0.6)
		v.content.Add(split)
		v.sftpBtn.SetText("SFTP ✓")
		v.tunnelBtn.SetText("隧道")
	case ViewModeTunnel:
		// 左右分割：终端 | 隧道
		split := container.NewHSplit(v.termBox, v.tunnelPnl)
		split.SetOffset(0.7)
		v.content.Add(split)
		v.sftpBtn.SetText("SFTP")
		v.tunnelBtn.SetText("隧道 ✓")
	}

	v.content.Refresh()
}

// OnSFTPBrowserReady 在 SSH 连接成功、SFTP 浏览器就绪时调用
func (v *TerminalView) OnSFTPBrowserReady(browser *sftp.Browser) {
	v.sftpView.SetBrowser(browser)
}

// SFTPView 返回 SFTP 面板（供外部设置回调）
func (v *TerminalView) SFTPView() *SFTPView { return v.sftpView }

// TunnelPanel 返回隧道面板（供外部设置回调）
func (v *TerminalView) TunnelPanel() *TunnelPanel { return v.tunnelPnl }

// FocusTerminal 将焦点设置到终端组件
func (v *TerminalView) FocusTerminal() {
	if v.win != nil && v.tab != nil {
		v.tab.FocusTerminal(v.win)
	}
}

// 确保 TerminalView 实现了 fyne.Widget 接口（编译期检查）
var _ fyne.Widget = (*TerminalView)(nil)
