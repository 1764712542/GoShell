package app

import (
	"fmt"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"

	"github.com/zhuyao/meatshell/internal/config"
	"github.com/zhuyao/meatshell/internal/i18n"
	"github.com/zhuyao/meatshell/internal/log"
)

// localMonitorTabID 是本机监控事件使用的特殊 tabID
const localMonitorTabID = "local"

// App 是应用主控制器，管理所有标签页、UI 事件循环和全局状态。
// 它连接 UI 层和后端模块，不直接依赖 UI 包（通过回调机制解耦）。
type App struct {
	fyneApp     fyne.App
	window      fyne.Window
	store       *config.Store
	prefs       *config.PreferencesManager
	stopWatcher func() error
	i18n        *i18n.Manager
	uiChan      chan UIEvent
	done        chan struct{} // 用于通知 eventLoop 退出
	closed      bool          // 标记是否已关闭，防止重复关闭

	tabs      map[string]*Tab
	activeTab string
	tabList   []string // 保持标签页顺序
	mu        sync.Mutex

	// 同步输入模式：开启后，在任一终端输入的命令会同步发送到所有已连接终端
	syncMode bool

	// UI 回调（由 UI 层设置，可为 nil）
	OnTabCreated      func(tab *Tab)       // 新标签页创建
	OnTabClosed       func(tabID string)   // 标签页关闭
	OnTabSwitched     func(tabID string)   // 标签页切换
	OnMetricsUpdate   func(m *MonitorData) // 本机监控指标更新
	OnAllTabsClosed   func()               // 所有标签页关闭（显示欢迎页）
	OnSyncModeChanged func(enabled bool)   // 同步输入模式切换
}

// New 创建应用控制器
func New(fyneApp fyne.App, store *config.Store, i18nMgr *i18n.Manager) *App {
	a := &App{
		fyneApp: fyneApp,
		store:   store,
		i18n:    i18nMgr,
		uiChan:  make(chan UIEvent, 2048),
		done:    make(chan struct{}),
		tabs:    make(map[string]*Tab),
	}
	a.initPreferences()
	return a
}

// initPreferences 创建配置管理器并启动 config.yaml 热重载。
// 加载失败时回退到默认配置，确保 prefs 永远非 nil。
func (a *App) initPreferences() {
	prefs, err := config.NewPreferencesManager()
	if err != nil {
		log.Warn("failed to load preferences, using defaults", "err", err)
		prefs = &config.PreferencesManager{}
		prefs.Update(config.DefaultPreferences())
	}
	a.prefs = prefs
	if stop, err := prefs.WatchConfigFile(); err != nil {
		log.Warn("failed to start config file watcher", "err", err)
	} else {
		a.stopWatcher = stop
	}
}

// FyneApp 返回 Fyne 应用实例
func (a *App) FyneApp() fyne.App { return a.fyneApp }

// Window 返回主窗口
func (a *App) Window() fyne.Window { return a.window }

// SetWindow 设置主窗口（由 UI 层的 BuildMainWindow 调用）
func (a *App) SetWindow(w fyne.Window) { a.window = w }

// Store 返回配置存储
func (a *App) Store() *config.Store { return a.store }

// Preferences 返回声明式配置管理器（支持热重载）
func (a *App) Preferences() *config.PreferencesManager { return a.prefs }

// I18n 返回国际化管理器
func (a *App) I18n() *i18n.Manager { return a.i18n }

// ActiveTab 返回当前活动的标签页（没有则返回 nil）
func (a *App) ActiveTab() *Tab {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.activeTab == "" {
		return nil
	}
	return a.tabs[a.activeTab]
}

// Tabs 返回所有标签页列表（按创建顺序）
func (a *App) Tabs() []*Tab {
	a.mu.Lock()
	defer a.mu.Unlock()
	result := make([]*Tab, 0, len(a.tabList))
	for _, id := range a.tabList {
		if tab, ok := a.tabs[id]; ok {
			result = append(result, tab)
		}
	}
	return result
}

// TabCount 返回当前标签页数量
func (a *App) TabCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.tabs)
}

// Run 启动 UI 事件循环并显示主窗口。
// 本机监控由 UI 层（window.go）负责启动。
// 此方法阻塞直到窗口关闭。
func (a *App) Run() {
	// 启动 UI 事件循环 goroutine
	go a.eventLoop()

	// 显示窗口并进入 Fyne 事件循环
	a.window.ShowAndRun()
}

// eventLoop 消费 uiChan 中的事件，分发到对应标签页。
// 所有 UI 操作通过 fyne.Do 在主线程执行。
// 通过 done channel 安全退出，不关闭 uiChan（避免后端 goroutine 发送到已关闭 channel 导致 panic）。
func (a *App) eventLoop() {
	for {
		select {
		case <-a.done:
			log.Info("event loop exited")
			return
		case evt := <-a.uiChan:
			// 本机监控事件（特殊 tabID）
			if evt.TabID == localMonitorTabID {
				a.handleLocalEvent(evt)
				continue
			}

			a.mu.Lock()
			tab, ok := a.tabs[evt.TabID]
			a.mu.Unlock()
			if !ok {
				continue
			}

			// 在主线程处理事件
			evtCopy := evt
			fyne.Do(func() {
				tab.HandleEvent(evtCopy)
			})

			// 需要窗口访问的特殊处理
			if evt.Type == EventStatus {
				a.handleStatusEvent(tab, evt)
			}
		}
	}
}

// handleLocalEvent 处理本机监控事件
func (a *App) handleLocalEvent(evt UIEvent) {
	if evt.Type == EventMonitor && evt.Metrics != nil {
		m := evt.Metrics
		fyne.Do(func() {
			if a.OnMetricsUpdate != nil {
				a.OnMetricsUpdate(m)
			}
		})
	}
}

// handleStatusEvent 处理需要窗口访问的状态事件（主机密钥确认、错误对话框）
func (a *App) handleStatusEvent(tab *Tab, evt UIEvent) {
	switch evt.Status {
	case StatusHostKeyPrompt:
		keyInfo := evt.HostKey
		fyne.Do(func() {
			a.showHostKeyDialog(tab, keyInfo)
		})
	case StatusError:
		msg := evt.StatusMsg
		fyne.Do(func() {
			dialog.ShowError(fmt.Errorf("%s", msg), a.window)
		})
	}
}

// showHostKeyDialog 显示主机密钥确认对话框
func (a *App) showHostKeyDialog(tab *Tab, keyInfo *HostKeyInfo) {
	if keyInfo == nil {
		return
	}
	title := "主机密钥确认"
	msg := fmt.Sprintf("主机: %s\n类型: %s\n指纹: %s\n\n首次连接此主机，是否信任并继续？",
		keyInfo.Host, keyInfo.KeyType, keyInfo.Fingerprint)
	dialog.ShowConfirm(title, msg, func(accepted bool) {
		tab.ConfirmHostKey(accepted)
	}, a.window)
}

// startLocalMonitor 已移至 UI 层（window.go），避免 app → monitor → app 循环依赖

// CreateSession 创建新会话标签页并启动连接
func (a *App) CreateSession(sess *config.Session) {
	tab := NewTab(sess, a.uiChan)
	if err := tab.Start(); err != nil {
		log.Error("failed to start tab", "err", err)
		dialog.ShowError(fmt.Errorf("启动会话失败: %w", err), a.window)
		return
	}

	a.mu.Lock()
	a.tabs[tab.ID] = tab
	a.tabList = append(a.tabList, tab.ID)
	a.activeTab = tab.ID
	a.mu.Unlock()

	// 通知 UI 层
	if a.OnTabCreated != nil {
		fyne.Do(func() {
			a.OnTabCreated(tab)
		})
	}

	// 聚焦终端
	fyne.Do(func() {
		tab.FocusTerminal(a.window)
	})
}

// CloseTab 关闭指定标签页
func (a *App) CloseTab(tabID string) {
	a.mu.Lock()
	tab, ok := a.tabs[tabID]
	if !ok {
		a.mu.Unlock()
		return
	}
	delete(a.tabs, tabID)
	// 从 tabList 中移除
	for i, id := range a.tabList {
		if id == tabID {
			a.tabList = append(a.tabList[:i], a.tabList[i+1:]...)
			break
		}
	}
	// 切换活动标签
	wasActive := a.activeTab == tabID
	if wasActive {
		if len(a.tabList) > 0 {
			a.activeTab = a.tabList[len(a.tabList)-1]
		} else {
			a.activeTab = ""
		}
	}
	newActive := a.activeTab
	hasTabs := len(a.tabList) > 0
	a.mu.Unlock()

	// 在锁外停止标签页（避免死锁）
	tab.Stop()

	// 通知 UI 层
	if a.OnTabClosed != nil {
		fyne.Do(func() {
			a.OnTabClosed(tabID)
		})
	}

	// 切换到新标签或显示欢迎页
	if wasActive {
		fyne.Do(func() {
			if hasTabs && newActive != "" {
				a.SwitchTab(newActive)
			} else if a.OnAllTabsClosed != nil {
				a.OnAllTabsClosed()
			}
		})
	}
}

// SwitchTab 切换到指定标签页
func (a *App) SwitchTab(tabID string) {
	a.mu.Lock()
	tab, ok := a.tabs[tabID]
	if !ok {
		a.mu.Unlock()
		return
	}
	a.activeTab = tabID
	a.mu.Unlock()

	// 通知 UI 层
	if a.OnTabSwitched != nil {
		fyne.Do(func() {
			a.OnTabSwitched(tabID)
		})
	}

	// 聚焦终端
	fyne.Do(func() {
		tab.FocusTerminal(a.window)
	})
}

// BroadcastCommand 向所有已连接的标签页广播命令
func (a *App) BroadcastCommand(cmd string) {
	data := []byte(cmd + "\n")
	a.mu.Lock()
	tabs := make([]*Tab, 0, len(a.tabs))
	for _, tab := range a.tabs {
		tabs = append(tabs, tab)
	}
	a.mu.Unlock()

	for _, tab := range tabs {
		tab.SendInput(data)
	}
	log.Info("broadcast command", "cmd", cmd, "tabs", len(tabs))
}

// SendCommand 向当前活动标签页发送命令。
// 如果同步输入模式开启，同时向所有已连接标签页发送。
func (a *App) SendCommand(cmd string) {
	data := []byte(cmd + "\n")
	if a.syncMode {
		a.BroadcastCommand(cmd)
		return
	}
	tab := a.ActiveTab()
	if tab == nil {
		return
	}
	tab.SendInput(data)
}

// SendInputToActive 向当前活动标签页发送原始输入（不自动加换行）。
// 如果同步输入模式开启，同时发送到所有已连接标签页。
func (a *App) SendInputToActive(data []byte) {
	if a.syncMode {
		a.mu.Lock()
		tabs := make([]*Tab, 0, len(a.tabs))
		for _, tab := range a.tabs {
			tabs = append(tabs, tab)
		}
		a.mu.Unlock()
		for _, tab := range tabs {
			tab.SendInput(data)
		}
		return
	}
	tab := a.ActiveTab()
	if tab == nil {
		return
	}
	tab.SendInput(data)
}

// SetSyncMode 设置同步输入模式
func (a *App) SetSyncMode(enabled bool) {
	a.mu.Lock()
	a.syncMode = enabled
	a.mu.Unlock()
	if a.OnSyncModeChanged != nil {
		fyne.Do(func() {
			a.OnSyncModeChanged(enabled)
		})
	}
	log.Info("sync mode changed", "enabled", enabled)
}

// IsSyncMode 返回同步输入模式是否开启
func (a *App) IsSyncMode() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.syncMode
}

// ToggleSyncMode 切换同步输入模式
func (a *App) ToggleSyncMode() {
	a.SetSyncMode(!a.syncMode)
}

// Shutdown 关闭所有标签页并清理资源。
// 通过关闭 done channel 通知 eventLoop 退出，不关闭 uiChan，
// 避免后端 goroutine 向已关闭 channel 发送数据导致 panic。
func (a *App) Shutdown() {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return
	}
	a.closed = true
	tabs := make([]*Tab, 0, len(a.tabs))
	for _, tab := range a.tabs {
		tabs = append(tabs, tab)
	}
	a.tabs = make(map[string]*Tab)
	a.tabList = nil
	a.activeTab = ""
	a.mu.Unlock()

	// 停止所有标签页（取消 context，后端 goroutine 会退出）
	for _, tab := range tabs {
		tab.Stop()
	}

	// 停止配置文件监听
	if a.stopWatcher != nil {
		a.stopWatcher()
	}
	// 通知 eventLoop 退出（不关闭 uiChan，避免 panic）
	close(a.done)
}

// UIChan 返回 UI 事件通道（供 UI 层创建本机监控器等使用）
func (a *App) UIChan() chan UIEvent { return a.uiChan }

// LocalMonitorTabID 返回本机监控事件使用的特殊 tabID
func (a *App) LocalMonitorTabID() string { return localMonitorTabID }
