package ui

import (
	"fmt"
	"image/color"
	"log"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/zhuyao/meatshell/internal/app"
	"github.com/zhuyao/meatshell/internal/sftp"
)

// TerminalView 是单个标签页的完整视图容器，组合终端组件、SFTP 面板和隧道面板。
// 通过顶部工具栏切换显示模式：仅终端 / 终端+SFTP / 终端+隧道。
type TerminalView struct {
	widget.BaseWidget
	tab         *app.Tab
	win         fyne.Window
	sftpView    *SFTPView
	tunnelPnl   *TunnelPanel
	termBox     *fyne.Container // 终端组件的容器
	content     *fyne.Container // 可切换的内容区域
	sftpBtn     *widget.Button
	tunnelBtn   *widget.Button
	mode        ViewMode
	errorState  *fyne.Container // 错误状态覆盖层
	errorLabel  *canvas.Text    // 错误消息文本
	onRetry     func()          // 重试回调（由 window.go 设置）
	onClose     func()          // 关闭回调（由 window.go 设置）
}

// ViewMode 表示终端视图的显示模式
type ViewMode int

const (
	ViewModeTerminal ViewMode = iota // 仅终端
	ViewModeSFTP                     // 终端 + SFTP
	ViewModeTunnel                   // 终端 + 隧道
)

// 毛玻璃效果颜色（多层半透明叠加，alpha 从 0x40 到 0x80）
var (
	colorFrostLayer1 = color.RGBA{R: 0x1e, G: 0x1e, B: 0x2e, A: 0x80} // 最底层，较深
	colorFrostLayer2 = color.RGBA{R: 0x30, G: 0x30, B: 0x44, A: 0x60} // 中间层
	colorFrostLayer3 = color.RGBA{R: 0x45, G: 0x47, B: 0x5a, A: 0x40} // 顶层，较浅
	colorErrorText   = color.RGBA{R: 0xed, G: 0x87, B: 0x96, A: 0xff} // 错误文本（红色）
	colorConnecting  = color.RGBA{R: 0xf9, G: 0xe2, B: 0xaf, A: 0xff} // 连接中文本（橙色）
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

	// 模式切换按钮（带图标）
	v.sftpBtn = widget.NewButtonWithIcon("SFTP", theme.FolderIcon(), func() {
		v.toggleMode(ViewModeSFTP)
	})
	v.tunnelBtn = widget.NewButtonWithIcon("隧道", theme.ViewRestoreIcon(), func() {
		v.toggleMode(ViewModeTunnel)
	})

	// 构建错误状态覆盖层
	v.buildErrorState()

	v.ExtendBaseWidget(v)
	return v
}

// buildErrorState 构建毛玻璃风格的错误状态覆盖层。
// 使用多层半透明 canvas.Rectangle 叠加模拟毛玻璃模糊效果。
func (v *TerminalView) buildErrorState() {
	// 错误图标（放大显示）
	iconCanvas := canvas.NewImageFromResource(theme.ErrorIcon())
	iconCanvas.FillMode = canvas.ImageFillContain
	iconCanvas.SetMinSize(fyne.NewSize(64, 64))

	// 错误消息文本
	v.errorLabel = canvas.NewText("连接失败", colorErrorText)
	v.errorLabel.TextSize = 18
	v.errorLabel.TextStyle = fyne.TextStyle{Bold: true}
	v.errorLabel.Alignment = fyne.TextAlignCenter

	// 重试按钮
	retryBtn := widget.NewButtonWithIcon("重试", theme.ViewRefreshIcon(), func() {
		if v.onRetry != nil {
			v.onRetry()
		}
	})
	retryBtn.Importance = widget.HighImportance

	// 关闭按钮
	closeBtn := widget.NewButtonWithIcon("关闭", theme.CancelIcon(), func() {
		if v.onClose != nil {
			v.onClose()
		}
	})

	// 按钮组（水平排列，有间距）
	buttons := container.NewHBox(retryBtn, closeBtn)

	// 居中内容（图标 + 文本 + 按钮）
	centerContent := container.NewVBox(
		iconCanvas,
		v.errorLabel,
		buttons,
	)

	// 多层半透明背景叠加，模拟毛玻璃效果
	// 从底层到顶层：最深的背景 → 中间层 → 浅层
	bgLayer1 := canvas.NewRectangle(colorFrostLayer1)
	bgLayer2 := canvas.NewRectangle(colorFrostLayer2)
	bgLayer3 := canvas.NewRectangle(colorFrostLayer3)

	// 叠加顺序：最底层 bgLayer1，中间 bgLayer2，顶层 bgLayer3，最上层是内容
	v.errorState = container.NewStack(
		bgLayer1,
		bgLayer2,
		bgLayer3,
		container.NewCenter(centerContent),
	)
	v.errorState.Hide()
}

// ShowError 显示毛玻璃风格的错误状态页面
func (v *TerminalView) ShowError(msg string) {
	if v.errorLabel != nil {
		v.errorLabel.Text = msg
		v.errorLabel.Refresh()
	}
	if v.errorState != nil {
		v.errorState.Show()
		v.errorState.Refresh()
	}
	v.Refresh()
}

// ShowConnecting 显示"正在连接..."的半透明覆盖层
func (v *TerminalView) ShowConnecting() {
	if v.errorLabel != nil {
		v.errorLabel.Text = "正在连接..."
		v.errorLabel.Color = colorConnecting
		v.errorLabel.Refresh()
	}
	if v.errorState != nil {
		v.errorState.Show()
		v.errorState.Refresh()
	}
	v.Refresh()
}

// ShowTerminal 显示正常终端（隐藏错误覆盖层）
func (v *TerminalView) ShowTerminal() {
	if v.errorLabel != nil {
		// 恢复错误文本颜色，供下次使用
		v.errorLabel.Color = colorErrorText
	}
	if v.errorState != nil {
		v.errorState.Hide()
		v.errorState.Refresh()
	}
	v.Refresh()
}

// SetOnRetry 设置重试回调
func (v *TerminalView) SetOnRetry(fn func()) { v.onRetry = fn }

// SetOnClose 设置关闭回调
func (v *TerminalView) SetOnClose(fn func()) { v.onClose = fn }

// CreateRenderer 实现 fyne.Widget 接口
func (v *TerminalView) CreateRenderer() fyne.WidgetRenderer {
	// 仅终端按钮（带图标）
	termBtn := widget.NewButtonWithIcon("仅终端", theme.DocumentIcon(), func() {
		v.setMode(ViewModeTerminal)
	})

	// 工具栏内容（靠右对齐：左侧用 Spacer 填充）
	toolbarContent := container.NewHBox(
		layout.NewSpacer(),
		v.sftpBtn,
		v.tunnelBtn,
		termBtn,
	)

	// 工具栏背景色
	toolbarBg := canvas.NewRectangle(colorToolbarBg)
	toolbar := container.NewStack(toolbarBg, container.NewPadded(toolbarContent))

	// 内容区域叠加错误状态覆盖层
	content := container.NewBorder(toolbar, nil, nil, nil,
		container.NewStack(v.content, v.errorState),
	)
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

// UpdateSFTPEntries 更新 SFTP 文件列表
func (v *TerminalView) UpdateSFTPEntries(entries []app.SFTPEntry) {
	if v.sftpView != nil {
		v.sftpView.Refresh()
	}
}

// SetSFTPCallbacks 设置 SFTP 回调（上传/下载/删除/创建目录）
// 必须在 SSH 连接成功后调用（此时 tab.SFTPClient() 才不为 nil）
func (v *TerminalView) SetSFTPCallbacks(a *app.App, tab *app.Tab) {
	if v.sftpView == nil {
		return
	}
	v.sftpView.SetOnUpload(func(localPath, remotePath string) {
		if client := tab.SFTPClient(); client != nil {
			go func() {
				if err := client.Upload(localPath, remotePath); err != nil {
					log.Printf("upload failed: %v", err)
				}
			}()
		}
	})
	v.sftpView.SetOnDownload(func(remotePath string) {
		if client := tab.SFTPClient(); client != nil {
			home, _ := os.UserHomeDir()
			localPath := filepath.Join(home, "Downloads", filepath.Base(remotePath))
			go func() {
				if err := client.Download(remotePath, localPath); err != nil {
					log.Printf("download failed: %v", err)
				}
			}()
		}
	})
	v.sftpView.SetOnMkdir(func(path string) {
		if client := tab.SFTPClient(); client != nil {
			go func() {
				if err := client.Mkdir(path); err != nil {
					log.Printf("mkdir failed: %v", err)
				}
			}()
		}
	})
	v.sftpView.SetOnRemove(func(path string) {
		if client := tab.SFTPClient(); client != nil {
			go func() {
				if err := client.Remove(path); err != nil {
					log.Printf("remove failed: %v", err)
				}
			}()
		}
	})
}

// SetTunnelCallbacks 设置隧道面板回调（添加/移除隧道）
// 必须在 SSH 连接成功后调用
func (v *TerminalView) SetTunnelCallbacks(tab *app.Tab) {
	if v.tunnelPnl == nil {
		return
	}
	// 设置窗口引用（用于弹出对话框）
	v.tunnelPnl.SetWindow(v.win)

	// 添加隧道回调
	v.tunnelPnl.SetOnAdd(func(typ, local, remote string) {
		if err := tab.AddTunnel(typ, local, remote); err != nil {
			dialog.ShowError(fmt.Errorf("添加隧道失败: %w", err), v.win)
		}
	})

	// 移除隧道回调（目前只停止所有隧道，后续可扩展为停止单个）
	v.tunnelPnl.SetOnRemove(func(index int) {
		// 暂时通过停止所有隧道来处理
		// 后续可以扩展为停止单个隧道
		dialog.ShowConfirm("移除隧道", "确定要移除此隧道吗？", func(ok bool) {
			if ok {
				v.tunnelPnl.RemoveTunnel(index)
				// TODO: 实现停止单个隧道
			}
		}, v.win)
	})
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
