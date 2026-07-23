package ui

import (
	"context"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"github.com/google/uuid"

	"github.com/zhuyao/meatshell/internal/app"
	"github.com/zhuyao/meatshell/internal/config"
	"github.com/zhuyao/meatshell/internal/monitor"
)

// MainWindow 是主窗口构建器，负责组装所有 UI 组件并连接 App 回调。
type MainWindow struct {
	app            *app.App
	win            fyne.Window
	theme          *Theme
	tabBar         *TabBar
	sidebar        *Sidebar
	welcome        *WelcomePage
	quickCmd       *QuickCmdBar
	content        *fyne.Container // 内容区域（welcome 或 tab views）
	views          map[string]*TerminalView
	localMon       *monitor.LocalMonitor
	localMonCtx    context.Context
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
	mw.win = a.FyneApp().NewWindow("GoShell")
	a.SetWindow(mw.win)
	mw.win.Resize(fyne.NewSize(1200, 800))
	mw.win.CenterOnScreen() // 窗口居中显示，更符合 macOS 启动体验

	// 创建各组件
	mw.tabBar = NewTabBar()
	mw.sidebar = NewSidebar()
	mw.welcome = NewWelcomePage(a.Store())
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
			view, ok := mw.views[tab.ID]
			if !ok {
				return
			}
			switch status {
			case app.StatusConnecting:
				view.ShowConnecting()
			case app.StatusError:
				view.ShowError(msg)
			case app.StatusConnected:
				view.ShowTerminal()
				// 连接成功后设置 SFTP 浏览器和回调
				if browser := tab.SFTPBrowser(); browser != nil {
					view.OnSFTPBrowserReady(browser)
				}
				// 延迟设置 SFTP 回调：此时 SFTP client 已创建
				view.SetSFTPCallbacks(mw.app, tab)
				// 设置隧道回调
				view.SetTunnelCallbacks(tab)
			case app.StatusDisconnected:
				view.ShowError("连接已断开")
			}
		}
		tab.OnSFTP = func(entries []app.SFTPEntry, progress *app.TransferProgress) {
			// SFTP 进度更新
			if progress != nil && !progress.Done {
				mw.win.SetTitle("GoShell - 传输中: " + progress.FileName)
			} else if progress != nil && progress.Done {
				activeTab := mw.app.ActiveTab()
				title := "GoShell"
				if activeTab != nil {
					title = "GoShell - " + activeTab.Session().Name
				}
				if mw.app.IsSyncMode() {
					title += " [同步输入]"
				}
				mw.win.SetTitle(title)
			}
			// 更新 SFTP 文件列表
			if entries != nil {
				view.UpdateSFTPEntries(entries)
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
			// 远端监控指标更新到侧边栏
			mw.sidebar.UpdateRemoteMetrics(metrics)
		}

		// 重连成功后重建 termBox：tab.tryReconnect 已替换 termWidget，
		// 需让 termBox 丢弃旧 widget 引用并装载新 widget（致命问题 #2）。
		// 回调在 goroutine 中触发，UI 操作必须经 fyne.Do 切回主线程。
		tab.OnReconnected = func(tab *app.Tab) {
			fyne.Do(func() {
				view, ok := mw.views[tab.ID]
				if !ok {
					return
				}
				view.RebuildTermBox()
				view.ShowConnecting()
			})
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
		mw.welcome.RefreshSessions()
		mw.content.RemoveAll()
		mw.content.Add(mw.welcome)
		mw.content.Refresh()
		mw.sidebar.UpdateStatus("就绪", false)
		mw.win.SetTitle("GoShell")
	}

	// 本机监控指标更新
	mw.app.OnMetricsUpdate = func(m *app.MonitorData) {
		mw.sidebar.UpdateMetrics(m)
	}

	// 同步输入模式切换
	mw.app.OnSyncModeChanged = func(enabled bool) {
		if enabled {
			mw.win.SetTitle(mw.win.Title() + " [同步输入]")
		} else {
			title := mw.win.Title()
			if idx := strings.Index(title, " [同步输入]"); idx >= 0 {
				mw.win.SetTitle(title[:idx])
			}
		}
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

	// 标签页复制
	mw.tabBar.SetOnDuplicate(func(index int) {
		tabs := mw.app.Tabs()
		if index < 0 || index >= len(tabs) {
			return
		}
		mw.app.CreateSession(tabs[index].Session())
	})

	// 标签页重命名（更新会话名称并刷新标签栏）
	mw.tabBar.SetOnRename(func(index int, name string) {
		tabs := mw.app.Tabs()
		if index < 0 || index >= len(tabs) {
			return
		}
		tab := tabs[index]
		sess := tab.Session()
		sess.Name = name
		if err := mw.app.Store().Update(sess); err == nil {
			mw.tabBar.UpdateTabTitle(index, name)
		}
	})

	// 标签页颜色标记
	mw.tabBar.SetOnSetColor(func(index int, color string) {
		mw.tabBar.UpdateTabColor(index, color)
	})

	// 关闭其他标签页
	mw.tabBar.SetOnCloseOthers(func(index int) {
		tabs := mw.app.Tabs()
		if index < 0 || index >= len(tabs) {
			return
		}
		keepID := tabs[index].ID
		for i := len(tabs) - 1; i >= 0; i-- {
			if tabs[i].ID != keepID {
				mw.app.CloseTab(tabs[i].ID)
			}
		}
	})

	// 关闭右侧所有标签页
	mw.tabBar.SetOnCloseRight(func(index int) {
		tabs := mw.app.Tabs()
		if index < 0 || index >= len(tabs) {
			return
		}
		for i := len(tabs) - 1; i > index; i-- {
			mw.app.CloseTab(tabs[i].ID)
		}
	})

	// 拖拽重排序
	mw.tabBar.SetOnReorder(func(from, to int) {
		mw.tabBar.MoveTab(from, to)
	})

	// 快捷命令栏回调
	mw.quickCmd.SetOnExecute(func(cmd string) {
		mw.app.SendCommand(cmd)
	})

	mw.quickCmd.SetOnBroadcast(func(cmd string) {
		mw.app.BroadcastCommand(cmd)
	})

	// 欢迎页回调：点击会话行 → 连接
	mw.welcome.SetOnConnect(func(sess *config.Session) {
		mw.app.CreateSession(sess)
	})

	// 欢迎页回调：新建会话
	mw.welcome.SetOnNewSession(func() {
		ShowNewSessionDialog(mw.app.Store(), mw.win, func(sess *config.Session) {
			mw.app.CreateSession(sess)
		})
	})

	// 欢迎页回调：编辑会话
	mw.welcome.SetOnEditSession(func(sess *config.Session) {
		d := NewSessionDialog(sess, mw.win, func(updated *config.Session) {
			if err := mw.app.Store().Update(updated); err != nil {
				dialog.ShowError(err, mw.win)
				return
			}
			mw.welcome.RefreshSessions()
		})
		d.Show()
	})

	// 欢迎页回调：删除会话
	mw.welcome.SetOnDeleteSession(func(sess *config.Session) {
		ShowDeleteConfirm(sess, mw.app.Store(), mw.win, func() {
			mw.welcome.RefreshSessions()
		})
	})

	// 本地终端快捷入口（一键打开，无需配置会话）
	mw.welcome.SetOnLocalTerm(func() {
		sess := config.NewSession("本地终端", config.SessionLocal)
		mw.app.CreateSession(sess)
	})

	mw.welcome.SetOnAbout(func() {
		dialog.ShowInformation("关于 GoShell",
			"GoShell v1.0.0\n\n"+
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

	// 欢迎页回调：复制会话（创建副本，名称后加 "_copy"）
	mw.welcome.SetOnDuplicateSession(func(sess *config.Session) {
		dup := *sess // 浅拷贝，敏感字段已处于解密态，Save 时会重新加密
		dup.ID = uuid.NewString()
		dup.Name = sess.Name + "_copy"
		dup.CreatedAt = time.Now()
		dup.UpdatedAt = time.Now()
		if err := mw.app.Store().Add(&dup); err != nil {
			dialog.ShowError(err, mw.win)
			return
		}
		mw.welcome.RefreshSessions()
	})

	// 欢迎页回调：移动会话到分组
	mw.welcome.SetOnMoveToGroup(func(sess *config.Session, group string) {
		updated := *sess
		updated.Group = group
		if err := mw.app.Store().Update(&updated); err != nil {
			dialog.ShowError(err, mw.win)
			return
		}
		mw.welcome.RefreshSessions()
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
		mw.tabBar,   // 顶部
		mw.quickCmd, // 底部
		nil,         // 左侧
		nil,         // 右侧
		mainArea,    // 中间
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

	// Ctrl+O: 刷新会话列表（会话列表已嵌入欢迎页）
	mw.win.Canvas().AddShortcut(&desktop.CustomShortcut{
		Modifier: fyne.KeyModifierControl,
		KeyName:  fyne.KeyO,
	}, func(_ fyne.Shortcut) {
		mw.welcome.RefreshSessions()
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

	// Ctrl+Shift+B: 切换同步输入模式
	mw.win.Canvas().AddShortcut(&desktop.CustomShortcut{
		Modifier: fyne.KeyModifierControl | fyne.KeyModifierShift,
		KeyName:  fyne.KeyB,
	}, func(_ fyne.Shortcut) {
		mw.app.ToggleSyncMode()
	})

	// Ctrl+Shift+L: 切换终端日志记录
	mw.win.Canvas().AddShortcut(&desktop.CustomShortcut{
		Modifier: fyne.KeyModifierControl | fyne.KeyModifierShift,
		KeyName:  fyne.KeyL,
	}, func(_ fyne.Shortcut) {
		tab := mw.app.ActiveTab()
		if tab == nil {
			return
		}
		tab.SetLogEnabled(!tab.IsLogEnabled())
		if tab.IsLogEnabled() {
			dialog.ShowInformation("终端日志", "日志已开启\n保存路径: "+tab.LogPath(), mw.win)
		}
	})

	// Ctrl+Shift+P: 打开配色方案选择
	mw.win.Canvas().AddShortcut(&desktop.CustomShortcut{
		Modifier: fyne.KeyModifierControl | fyne.KeyModifierShift,
		KeyName:  fyne.KeyP,
	}, func(_ fyne.Shortcut) {
		ShowColorSchemeDialog(mw.win, "", func(scheme *ColorScheme) {
			// 应用配色方案（终端组件会在下次创建时使用新配色）
			mw.welcome.RefreshSessions()
		})
	})

	// Ctrl+Shift+F: 终端搜索
	mw.win.Canvas().AddShortcut(&desktop.CustomShortcut{
		Modifier: fyne.KeyModifierControl | fyne.KeyModifierShift,
		KeyName:  fyne.KeyF,
	}, func(_ fyne.Shortcut) {
		tab := mw.app.ActiveTab()
		if tab == nil {
			return
		}
		_, ok := mw.views[tab.ID]
		if !ok {
			return
		}
		// 调用终端 widget 的 ToggleSearch 方法
		if tw := tab.TermWidget(); tw != nil {
			tw.ToggleSearch()
		}
	})

	// Ctrl+Shift+S: 打开设置
	mw.win.Canvas().AddShortcut(&desktop.CustomShortcut{
		Modifier: fyne.KeyModifierControl | fyne.KeyModifierShift,
		KeyName:  fyne.KeyS,
	}, func(_ fyne.Shortcut) {
		ShowSettingsDialog(mw.win, mw.app.Store(), mw.app.Preferences())
	})

	// Ctrl+Shift+D: 水平拆分当前终端 pane（左右）
	mw.win.Canvas().AddShortcut(&desktop.CustomShortcut{
		Modifier: fyne.KeyModifierControl | fyne.KeyModifierShift,
		KeyName:  fyne.KeyD,
	}, func(_ fyne.Shortcut) {
		if view := mw.activeTerminalView(); view != nil {
			view.SplitHorizontal()
		}
	})

	// Alt+Shift+D: 垂直拆分当前终端 pane（上下）
	mw.win.Canvas().AddShortcut(&desktop.CustomShortcut{
		Modifier: fyne.KeyModifierAlt | fyne.KeyModifierShift,
		KeyName:  fyne.KeyD,
	}, func(_ fyne.Shortcut) {
		if view := mw.activeTerminalView(); view != nil {
			view.SplitVertical()
		}
	})

	// Ctrl+Shift+W: 关闭当前 pane（若已拆分则关闭镜像 pane，否则关闭标签页）
	mw.win.Canvas().AddShortcut(&desktop.CustomShortcut{
		Modifier: fyne.KeyModifierControl | fyne.KeyModifierShift,
		KeyName:  fyne.KeyW,
	}, func(_ fyne.Shortcut) {
		view := mw.activeTerminalView()
		if view != nil && view.HasSplit() {
			view.ClosePane()
			return
		}
		tab := mw.app.ActiveTab()
		if tab != nil {
			mw.app.CloseTab(tab.ID)
		}
	})
}

// activeTerminalView 返回当前激活标签页对应的 TerminalView，没有时返回 nil。
func (mw *MainWindow) activeTerminalView() *TerminalView {
	tab := mw.app.ActiveTab()
	if tab == nil {
		return nil
	}
	return mw.views[tab.ID]
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

	// 延迟聚焦终端：通过 goroutine + fyne.Do 将焦点设置推迟到下一个主线程周期，
	// 确保内容区域布局完成后再聚焦，避免 Fyne 在组件未完成布局时焦点失效。
	go func() {
		fyne.Do(func() {
			view.FocusTerminal()
		})
	}()

	// 更新窗口标题
	tab := mw.app.ActiveTab()
	if tab != nil {
		mw.win.SetTitle("GoShell - " + tab.Session().Name)
	}
}

// rebuildTabBar 根据当前 App 中的标签页批量重建标签栏
func (mw *MainWindow) rebuildTabBar() {
	tabs := mw.app.Tabs()
	items := make([]TabItem, len(tabs))
	for i, tab := range tabs {
		items[i] = TabItem{
			ID:       tab.ID,
			Title:    tab.Session().Name,
			Closable: true,
		}
	}
	// 使用 SetTabs 批量设置，避免逐个 AddTab 导致的 O(n²) 重建
	// 同时使用 SetActiveSilent 避免触发不必要的标签切换
	activeTab := mw.app.ActiveTab()
	activeID := ""
	if activeTab != nil {
		activeID = activeTab.ID
	}
	mw.tabBar.SetTabs(items, activeID)
}

// syncTabBarActive 同步标签栏的选中状态（不触发 onChange，避免循环调用）
func (mw *MainWindow) syncTabBarActive(tabID string) {
	tabs := mw.app.Tabs()
	for i, tab := range tabs {
		if tab.ID == tabID {
			mw.tabBar.SetActiveSilent(i)
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
