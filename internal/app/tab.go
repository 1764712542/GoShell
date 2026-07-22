// Package app 提供应用主控制器和标签页管理。
// App 连接 UI 层和后端模块（ssh/serial/telnet/terminal/sftp/monitor），
// 通过 uiChan 实现后端 goroutine 到 UI 主线程的事件传递。
package app

import (
	"context"
	"fmt"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/zhuyao/meatshell/internal/config"
	"github.com/zhuyao/meatshell/internal/log"
	"github.com/zhuyao/meatshell/internal/monitor"
	"github.com/zhuyao/meatshell/internal/serial"
	"github.com/zhuyao/meatshell/internal/sftp"
	"github.com/zhuyao/meatshell/internal/ssh"
	"github.com/zhuyao/meatshell/internal/telnet"
	"github.com/zhuyao/meatshell/internal/terminal"
)

// Tab 管理单个会话标签页的完整生命周期，包括连接建立、
// 终端渲染、SFTP 浏览、远端监控和资源清理。
type Tab struct {
	ID         string
	session    *config.Session
	container  fyne.CanvasObject
	worker     interface{} // *ssh.Worker / *serial.Worker / *telnet.Worker
	emulator   *terminal.Emulator
	termWidget *terminal.TerminalWidget
	ctx        context.Context
	cancel     context.CancelFunc

	sftpClient  *sftp.Client
	sftpBrowser *sftp.Browser
	remoteMon   *monitor.RemoteMonitor
	processMon  *monitor.ProcessMonitor

	uiChan    chan UIEvent
	connected bool
	closed    bool
	mu        sync.Mutex

	// UI 组件
	statusBar *widget.Label

	// UI 回调（由 UI 层设置，可为 nil）
	OnStatus  func(status ConnectionStatus, msg string)
	OnSFTP    func(entries []SFTPEntry, progress *TransferProgress)
	OnTunnel  func(status *TunnelStatus)
	OnMonitor func(metrics *MonitorData)
}

// NewTab 创建一个新的标签页。uiChan 用于接收后端事件。
func NewTab(sess *config.Session, uiChan chan UIEvent) *Tab {
	return &Tab{
		ID:      sess.ID,
		session: sess,
		uiChan:  uiChan,
	}
}

// Session 返回标签页关联的会话配置
func (t *Tab) Session() *config.Session { return t.session }

// TermWidget 返回终端组件（供 UI 层创建 TerminalView）
func (t *Tab) TermWidget() *terminal.TerminalWidget { return t.termWidget }

// Emulator 返回终端模拟器
func (t *Tab) Emulator() *terminal.Emulator { return t.emulator }

// SFTPBrowser 返回 SFTP 浏览器（连接成功后可用）
func (t *Tab) SFTPBrowser() *sftp.Browser { return t.sftpBrowser }

// SFTPClient 返回 SFTP 客户端（连接成功后可用）
func (t *Tab) SFTPClient() *sftp.Client { return t.sftpClient }

// IsConnected 返回当前是否已连接
func (t *Tab) IsConnected() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.connected
}

// Container 返回标签页的显示内容
func (t *Tab) Container() fyne.CanvasObject { return t.container }

// SetContainer 设置标签页的显示内容（由 UI 层调用）
func (t *Tab) SetContainer(c fyne.CanvasObject) { t.container = c }

// Start 启动会话：创建终端、worker 并发起连接。
func (t *Tab) Start() error {
	t.ctx, t.cancel = context.WithCancel(context.Background())

	// 创建终端模拟器（80x24）
	t.emulator = terminal.NewEmulator(80, 24)

	switch t.session.Type {
	case config.SessionSSH:
		return t.startSSH()
	case config.SessionSerial:
		return t.startSerial()
	case config.SessionTelnet:
		return t.startTelnet()
	default:
		return fmt.Errorf("unsupported session type: %s", t.session.Type)
	}
}

// startSSH 启动 SSH 会话
func (t *Tab) startSSH() error {
	worker := ssh.NewWorker(t.session, t.uiChan)
	t.worker = worker

	// 创建终端组件，键盘输入转发到 SSH
	t.termWidget = terminal.NewTerminalWidget(t.emulator, worker.SendInput)

	// 构建默认容器
	t.buildDefaultContainer()

	// 异步发起连接
	go func() {
		if err := worker.Connect(t.ctx); err != nil {
			log.Warn("ssh connect failed", "host", t.session.Host, "err", err)
		}
	}()
	return nil
}

// startSerial 启动串口会话
// 约定：session.Host 存储串口设备名，session.Port 存储波特率
func (t *Tab) startSerial() error {
	baudRate := t.session.Port
	if baudRate == 0 {
		baudRate = 115200
	}
	worker := serial.NewWorker(t.session.Host, baudRate, t.uiChan, t.ID)
	t.worker = worker

	t.termWidget = terminal.NewTerminalWidget(t.emulator, worker.SendInput)
	t.buildDefaultContainer()

	go func() {
		if err := worker.Connect(t.ctx); err != nil {
			log.Warn("serial connect failed", "port", t.session.Host, "err", err)
		}
	}()
	return nil
}

// startTelnet 启动 Telnet 会话
func (t *Tab) startTelnet() error {
	port := t.session.Port
	if port == 0 {
		port = 23
	}
	worker := telnet.NewWorker(t.session.Host, port, t.uiChan, t.ID)
	t.worker = worker

	t.termWidget = terminal.NewTerminalWidget(t.emulator, worker.SendInput)
	t.buildDefaultContainer()

	go func() {
		if err := worker.Connect(t.ctx); err != nil {
			log.Warn("telnet connect failed", "host", t.session.Host, "err", err)
		}
	}()
	return nil
}

// buildDefaultContainer 构建默认显示容器：终端居中 + 底部状态栏
func (t *Tab) buildDefaultContainer() {
	t.statusBar = widget.NewLabel("就绪")
	t.statusBar.Wrapping = fyne.TextWrapOff
	t.statusBar.TextStyle = fyne.TextStyle{Monospace: true}
	t.container = container.NewBorder(nil, t.statusBar, nil, nil, t.termWidget)
}

// startPostConnect 在 SSH 连接成功后启动 SFTP、远端监控和进程监控
func (t *Tab) startPostConnect() {
	sshWorker, ok := t.worker.(*ssh.Worker)
	if !ok {
		return
	}
	client := sshWorker.Client()
	if client == nil {
		return
	}

	// 创建 SFTP 客户端
	sftpClient, err := sftp.NewClient(client, t.uiChan, t.ID)
	if err != nil {
		log.Warn("create sftp client failed", "err", err)
	} else {
		t.sftpClient = sftpClient
		t.sftpBrowser = sftp.NewBrowser(sftpClient)
	}

	// 启动远端系统监控（2 秒采样间隔）
	t.remoteMon = monitor.NewRemoteMonitor(client, t.uiChan, t.ID, 2*time.Second)
	go t.remoteMon.Start(t.ctx)

	// 启动进程监控（5 秒采样间隔）
	t.processMon = monitor.NewProcessMonitor(client, t.uiChan, t.ID)
	go t.processMon.Start(t.ctx, 5*time.Second)
}

// HandleEvent 处理从 uiChan 接收的事件。
// 此方法在主线程中调用（通过 fyne.Do）。
func (t *Tab) HandleEvent(evt UIEvent) {
	switch evt.Type {
	case EventTerminal:
		if len(evt.TerminalData) > 0 {
			t.emulator.Write(evt.TerminalData)
			t.termWidget.TriggerRefresh()
		}

	case EventStatus:
		// 更新状态栏
		if t.statusBar != nil {
			t.statusBar.SetText(evt.StatusMsg)
		}
		// 连接状态变更
		switch evt.Status {
		case StatusConnected:
			t.mu.Lock()
			t.connected = true
			t.mu.Unlock()
			t.startPostConnect()
		case StatusDisconnected:
			t.mu.Lock()
			t.connected = false
			t.mu.Unlock()
		}
		// 通知 UI 层
		if t.OnStatus != nil {
			t.OnStatus(evt.Status, evt.StatusMsg)
		}

	case EventMonitor:
		if t.OnMonitor != nil {
			t.OnMonitor(evt.Metrics)
		}

	case EventSFTP:
		if t.OnSFTP != nil {
			t.OnSFTP(evt.SFTPList, evt.SFTPProgress)
		}

	case EventTunnel:
		if t.OnTunnel != nil {
			t.OnTunnel(evt.TunnelStatus)
		}

	case EventProcess:
		// 进程列表暂不展示，预留扩展
	}
}

// Stop 停止会话并清理所有资源（goroutine、连接、SFTP 等）。
// 此方法是幂等的，可安全多次调用。
func (t *Tab) Stop() {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return
	}
	t.closed = true
	t.mu.Unlock()

	// 停止远端监控
	if t.remoteMon != nil {
		t.remoteMon.Stop()
	}
	if t.processMon != nil {
		t.processMon.Stop()
	}

	// 关闭 SFTP 客户端
	if t.sftpClient != nil {
		t.sftpClient.Close()
	}

	// 取消 context
	if t.cancel != nil {
		t.cancel()
	}

	// 关闭 worker（根据类型断言）
	switch w := t.worker.(type) {
	case *ssh.Worker:
		w.Close()
	case *serial.Worker:
		w.Close()
	case *telnet.Worker:
		w.Close()
	}

	log.Info("tab stopped", "id", t.ID, "session", t.session.Name)
}

// SendInput 向会话发送输入数据
func (t *Tab) SendInput(data []byte) {
	switch w := t.worker.(type) {
	case *ssh.Worker:
		w.SendInput(data)
	case *serial.Worker:
		w.SendInput(data)
	case *telnet.Worker:
		w.SendInput(data)
	}
}

// Resize 调整终端窗口大小（仅 SSH 支持）
func (t *Tab) Resize(cols, rows int) {
	if w, ok := t.worker.(*ssh.Worker); ok {
		if err := w.Resize(cols, rows); err != nil {
			log.Warn("resize terminal failed", "err", err)
		}
	}
	t.termWidget.SetSize(cols, rows)
}

// ConfirmHostKey 响应主机密钥确认请求（由 App 在对话框回调中调用）
func (t *Tab) ConfirmHostKey(accepted bool) {
	if w, ok := t.worker.(*ssh.Worker); ok {
		w.ConfirmHostKey(accepted)
	}
}

// FocusTerminal 将焦点设置到终端组件
func (t *Tab) FocusTerminal(window fyne.Window) {
	if t.termWidget != nil && window != nil {
		window.Canvas().Focus(t.termWidget)
	}
}

// AddTunnel 添加 SSH 隧道
func (t *Tab) AddTunnel(tunnelType, localAddr, remoteAddr string) error {
	w, ok := t.worker.(*ssh.Worker)
	if !ok {
		return fmt.Errorf("tunnels are only supported for SSH sessions")
	}
	switch tunnelType {
	case "local":
		return w.LocalForward(localAddr, remoteAddr)
	case "remote":
		return w.RemoteForward(remoteAddr, localAddr)
	case "dynamic":
		return w.DynamicForward(localAddr)
	default:
		return fmt.Errorf("unknown tunnel type: %s", tunnelType)
	}
}

// StopTunnels 停止所有 SSH 隧道
func (t *Tab) StopTunnels() {
	if w, ok := t.worker.(*ssh.Worker); ok {
		w.StopTunnels()
	}
}
