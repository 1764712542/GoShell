package ui

import (
	"context"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"

	"github.com/zhuyao/meatshell/internal/app"
	"github.com/zhuyao/meatshell/internal/config"
	"github.com/zhuyao/meatshell/internal/monitor"
)

// MainWindow 是主窗口构建器，负责组装所有 UI 组件并连接 App 回调。
type MainWindow struct {
	app          *app.App
	win          fyne.Window
	theme        *Theme
	tabBar       *TabBar
	sidebar      *Sidebar
	welcome      *WelcomePage
	quickCmd     *QuickCmdBar
	content      *fyne.Container // 内容区域（welcome 或 tab views）
	views        map[string]*TerminalView
	localMon     *monitor.LocalMonitor
	localMonCtx  context.Context
	localMonCancel context.CancelFunc
}

// NewMainWindow 创建主窗口构建器
func NewMainWindow(a *app.App) *MainWindow {
	mw := &MainWindow{
		app:   a,
		views: make(map[string]*TerminalView),
	}

	// 创建主题
	mw.theme = NewTheme(a.FyneApp().Preferences())
	a.FyneApp().Settings().SetTheme(mw.theme)

	// 创建主窗口
	mw.win = a.FyneApp().NewWindow("Meatshell")
	a.SetWindow(mw.win)
	mw.win.Resize(fyne.NewSize(1200, 800))

	// 创建各组件
	mw.tabBar = NewTabBar()
	mw.sidebar = NewSidebar()
	mw.welcome = NewWelcomePage()
	mw.quickCmd = NewQuickCmdBar()
	mw.content = container.NewStack(mw.welcome)

	// 设置 App 回调
	mw.setupAppCallbacks()

	// 设置组件回调
	mw.setupComponentCallbacks()

	// 设置窗口内容
	mw.buildWindowContent()

	// 设置快捷键
	mw.setupShortcuts()

	// 设置窗口关闭处理
	mw.win.SetCloseIntercept(func() {
		mw.handleClose()
	})

	return mw
}

// setupAppCallbacks 设置 App 层的回调
func (mw *MainWindow) setupAppCallbacks() {
	// 新标签页创建
	mw.app.OnTabCreated = func(tab *app.Tab) {
		view := NewTerminalView(tab, mw.win)
		mw.views[tab.ID] = view

		// 设置 tab 的 UI 回调
		tab.OnStatus = func(status app.ConnectionStatus, msg string) {
			mw.sidebar.UpdateStatus(msg, status == app.StatusConnected)
		}
		tab.OnSFTP = func(entries []app.SFTPEntry, progress *app.TransferProgress) {
			// SFTP 进度更新（可扩展显示进度条）
			if progress != nil && !progress.Done {
				mw.win.SetTitle("Meatshell - 传输中: " + progress.FileName)
			} else if progress != nil && progress.Done {
				mw.win.SetTitle("Meatshell")
			}
		}
		tab.OnTunnel = func(status *app.TunnelStatus) {
			if status != nil && view.TunnelPanel() != nil {
				view.TunnelPanel().AddTunnel(TunnelRow{
					Type:       status.Type,
					LocalAddr:  status.LocalAddr,
					RemoteAddr: status.RemoteAddr,
					Active:     status.Active,
					Error:      status.Error,
				})
			}
		}
		tab.OnMonitor = func(metrics *app.MonitorData) {
			// 远端监控指标（可扩展显示在侧边栏）
		}

		// 添加到标签栏
		mw.tabBar.AddTab(TabItem{
			ID:       tab.ID,
			Title:    tab.Session().Name,
			Closable: true,
		})

		// 切换到新标签页
		mw.showTab(tab.ID)
	}

	// 标签页关闭
	mw.app.OnTabClosed = func(tabID string) {
		delete(mw.views, tabID)
		// 找到对应的标签索引并移除
		for i := 0; i < mw.tabBar.GetTabCount(); i++ {
			// 由于 TabBar 没有提供按 ID 查找的方法，需要通过重建来同步
			// 这里简单清空并重建
		}
		mw.rebuildTabBar()
	}

	// 标签页切换
	mw.app.OnTabSwitched = func(tabID string) {
		mw.showTab(tabID)
		// 同步标签栏选中状态
		mw.syncTabBarActive(tabID)
	}

	// 所有标签页关闭
	mw.app.OnAllTabsClosed = func() {
		mw.content.RemoveAll()
		mw.content.Add(mw.welcome)
		mw.content.Refresh()
		mw.sidebar.UpdateStatus("就绪", false)
		mw.win.SetTitle("Meatshell")
	}

	// 本机监控指标更新
	mw.app.OnMetricsUpdate = func(m *app.MonitorData) {
		mw.sidebar.UpdateMetrics(m)
	}
}

// setupComponentCallbacks 设置 UI 组件的回调
func (mw *MainWindow) setupComponentCallbacks() {
	// 标签栏回调
	mw.tabBar.SetOnChange(func(index int) {
		tabs := mw.app.Tabs()
		if index >= 0 && index < len(tabs) {
			mw.app.SwitchTab(tabs[index].ID)
		}
	})

	mw.tabBar.SetOnAdd(func() {
		ShowNewSessionDialog(mw.app.Store(), mw.win, func(sess *config.Session) {
			mw.app.CreateSession(sess)
		})
	})

	mw.tabBar.SetOnClose(func(index int) {
		tabs := mw.app.Tabs()
		if index >= 0 && index < len(tabs) {
			mw.app.CloseTab(tabs[index].ID)
		}
	})

	// 快捷命令栏回调
	mw.quickCmd.SetOnExecute(func(cmd string) {
		mw.app.SendCommand(cmd)
	})

	mw.quickCmd.SetOnBroadcast(func(cmd string) {
		mw.app.BroadcastCommand(cmd)
	})

	// 欢迎页回调
	mw.welcome.SetOnNewSession(func() {
		ShowNewSessionDialog(mw.app.Store(), mw.win, func(sess *config.Session) {
			mw.app.CreateSession(sess)
		})
	})

	mw.welcome.SetOnOpenSession(func() {
		ShowSessionListDialog(mw.app.Store(), mw.win, func(sess *config.Session) {
			mw.app.CreateSession(sess)
		})
	})

	mw.welcome.SetOnAbout(func() {
		dialog.ShowInformation("关于 Meatshell",
			"Meatshell v1.0.0\n\n"+
				"SSH / 串口 / Telnet 终端客户端\n"+
				"基于 Fyne v2 构建\n\n"+
				"功能：\n"+
				"• SSH/Serial/Telnet 连接\n"+
				"• SFTP 文件传输\n"+
				"• SSH 隧道（本地/远程/动态）\n"+
				"• 系统监控（本机/远端）\n"+
				"• 多标签页管理\n"+
				"• 命令广播", mw.win)
	})
}

// buildWindowContent 构建主窗口内容布局
func (mw *MainWindow) buildWindowContent() {
	// 顶部：标签栏
	// 左侧：系统监控侧边栏
	// 中间：内容区域（welcome 或 tab views）
	// 底部：快捷命令栏

	// 内容区域 + 侧边栏（水平分割）
	mainArea := container.NewHSplit(mw.sidebar, mw.content)
	mainArea.SetOffset(0.15)

	// 顶部标签栏 + 主区域 + 底部命令栏
	layout := container.NewBorder(
		mw.tabBar,    // 顶部
		mw.quickCmd,  // 底部
		nil,          // 左侧
		nil,          // 右侧
		mainArea,     // 中间
	)

	mw.win.SetContent(layout)
}

// setupShortcuts 设置全局快捷键
func (mw *MainWindow) setupShortcuts() {
	// Ctrl+N: 新建会话
	mw.win.Canvas().AddShortcut(&desktop.CustomShortcut{
		Modifier: fyne.KeyModifierControl,
		KeyName:  fyne.KeyN,
	}, func(_ fyne.Shortcut) {
		ShowNewSessionDialog(mw.app.Store(), mw.win, func(sess *config.Session) {
			mw.app.CreateSession(sess)
		})
	})

	// Ctrl+O: 打开会话列表
	mw.win.Canvas().AddShortcut(&desktop.CustomShortcut{
		Modifier: fyne.KeyModifierControl,
		KeyName:  fyne.KeyO,
	}, func(_ fyne.Shortcut) {
		ShowSessionListDialog(mw.app.Store(), mw.win, func(sess *config.Session) {
			mw.app.CreateSession(sess)
		})
	})

	// Ctrl+W: 关闭当前标签
	mw.win.Canvas().AddShortcut(&desktop.CustomShortcut{
		Modifier: fyne.KeyModifierControl,
		KeyName:  fyne.KeyW,
	}, func(_ fyne.Shortcut) {
		tab := mw.app.ActiveTab()
		if tab != nil {
			mw.app.CloseTab(tab.ID)
		}
	})

	// Ctrl+T: 切换主题
	mw.win.Canvas().AddShortcut(&desktop.CustomShortcut{
		Modifier: fyne.KeyModifierControl,
		KeyName:  fyne.KeyT,
	}, func(_ fyne.Shortcut) {
		mw.theme.ToggleVariant()
		mw.app.FyneApp().Settings().SetTheme(mw.theme)
	})
}

// showTab 显示指定标签页的内容
func (mw *MainWindow) showTab(tabID string) {
	view, ok := mw.views[tabID]
	if !ok {
		return
	}

	mw.content.RemoveAll()
	mw.content.Add(view)
	mw.content.Refresh()

	// 聚焦终端
	view.FocusTerminal()

	// 更新窗口标题
	tab := mw.app.ActiveTab()
	if tab != nil {
		mw.win.SetTitle("Meatshell - " + tab.Session().Name)
	}
}

// rebuildTabBar 根据当前 App 中的标签页重建标签栏
func (mw *MainWindow) rebuildTabBar() {
	mw.tabBar.Clear()
	tabs := mw.app.Tabs()
	for _, tab := range tabs {
		mw.tabBar.AddTab(TabItem{
			ID:       tab.ID,
			Title:    tab.Session().Name,
			Closable: true,
		})
	}
}

// syncTabBarActive 同步标签栏的选中状态
func (mw *MainWindow) syncTabBarActive(tabID string) {
	tabs := mw.app.Tabs()
	for i, tab := range tabs {
		if tab.ID == tabID {
			mw.tabBar.SetActive(i)
			return
		}
	}
}

// startLocalMonitor 启动本机系统监控
func (mw *MainWindow) startLocalMonitor() {
	mw.localMonCtx, mw.localMonCancel = context.WithCancel(context.Background())
	mw.localMon = monitor.NewLocalMonitor(
		mw.app.UIChan(),
		mw.app.LocalMonitorTabID(),
		2*time.Second,
	)
	go mw.localMon.Start(mw.localMonCtx)
}

// stopLocalMonitor 停止本机系统监控
func (mw *MainWindow) stopLocalMonitor() {
	if mw.localMon != nil {
		mw.localMon.Stop()
	}
	if mw.localMonCancel != nil {
		mw.localMonCancel()
	}
}

// handleClose 处理窗口关闭事件
func (mw *MainWindow) handleClose() {
	// 停止本机监控
	mw.stopLocalMonitor()

	// 关闭所有标签页
	mw.app.Shutdown()

	// 关闭窗口
	mw.win.Close()
}

// Run 显示窗口并启动应用
func (mw *MainWindow) Run() {
	// 启动本机监控
	mw.startLocalMonitor()

	// 启动 App（包含事件循环）
	mw.app.Run()
}

// Window 返回主窗口实例
func (mw *MainWindow) Window() fyne.Window { return mw.win }

// Show 显示主窗口
func (mw *MainWindow) Show() {
	mw.win.Show()
}
