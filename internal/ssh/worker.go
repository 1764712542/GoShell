package ssh

import (
	"context"
	"fmt"
	"io"
	"sync"

	"golang.org/x/crypto/ssh"

	"github.com/zhuyao/meatshell/internal/config"
	"github.com/zhuyao/meatshell/internal/event"
	"github.com/zhuyao/meatshell/internal/log"
)

// Worker 管理一个 SSH 会话的完整生命周期，包括连接建立、
// 认证、PTY 交互、数据读写和端口转发。
type Worker struct {
	session   *config.Session
	uiChan    chan event.UIEvent // 发送给 UI 的事件通道
	inputChan chan []byte       // 接收键盘输入的通道

	client     *ssh.Client
	sshSession *ssh.Session
	stdin      io.WriteCloser

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu        sync.Mutex
	connected bool
	closed    bool
	cols      int
	rows      int

	// 主机密钥确认回调
	hostKeyConfirm chan bool

	// 活跃的隧道列表
	tunnels []*tunnelInfo
}

// NewWorker 创建一个新的 SSH Worker。
// uiChan 用于向 UI 发送事件，inputChan 用于接收键盘输入。
func NewWorker(sess *config.Session, uiChan chan event.UIEvent) *Worker {
	return &Worker{
		session:        sess,
		uiChan:         uiChan,
		inputChan:      make(chan []byte, 256),
		hostKeyConfirm: make(chan bool, 1),
		cols:           80,
		rows:           24,
	}
}

// Connect 建立 SSH 连接并启动交互式 Shell 会话。
// 流程：
//  1. 通过代理（如有）或直连建立 TCP 连接
//  2. 完成 SSH 握手和认证
//  3. 创建 Session，请求 PTY 和 Shell
//  4. 启动读取和输入 goroutine
//  5. 启动配置的隧道（如有）
func (w *Worker) Connect(ctx context.Context) error {
	w.mu.Lock()
	if w.connected {
		w.mu.Unlock()
		return fmt.Errorf("already connected")
	}
	w.mu.Unlock()

	// 创建可取消的 context
	w.ctx, w.cancel = context.WithCancel(ctx)

	// 发送连接中状态
	w.sendStatus(event.StatusConnecting, fmt.Sprintf("正在连接 %s:%d...", w.session.Host, w.session.Port))

	// 建立 SSH 连接
	client, err := w.dial(w.ctx)
	if err != nil {
		w.sendStatus(event.StatusError, fmt.Sprintf("连接失败: %v", err))
		return fmt.Errorf("ssh connect: %w", err)
	}

	w.mu.Lock()
	w.client = client
	w.connected = true
	w.mu.Unlock()

	// 创建 SSH Session
	session, err := client.NewSession()
	if err != nil {
		w.closeConnections()
		w.sendStatus(event.StatusError, fmt.Sprintf("创建会话失败: %v", err))
		return fmt.Errorf("new ssh session: %w", err)
	}

	// 获取 stdin 管道（必须在 Shell 之前调用）
	stdin, err := session.StdinPipe()
	if err != nil {
		session.Close()
		w.closeConnections()
		w.sendStatus(event.StatusError, fmt.Sprintf("获取输入管道失败: %v", err))
		return fmt.Errorf("stdin pipe: %w", err)
	}

	// 获取 stdout 管道（必须在 Shell 之前调用）
	stdout, err := session.StdoutPipe()
	if err != nil {
		stdin.Close()
		session.Close()
		w.closeConnections()
		w.sendStatus(event.StatusError, fmt.Sprintf("获取输出管道失败: %v", err))
		return fmt.Errorf("stdout pipe: %w", err)
	}

	w.mu.Lock()
	w.sshSession = session
	w.stdin = stdin
	w.mu.Unlock()

	// 请求 PTY
	termType := w.session.TermType
	if termType == "" {
		termType = "xterm-256color"
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          1, // 启用回显
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	w.mu.Lock()
	cols, rows := w.cols, w.rows
	w.mu.Unlock()

	if err := session.RequestPty(termType, rows, cols, modes); err != nil {
		w.closeConnections()
		w.sendStatus(event.StatusError, fmt.Sprintf("请求 PTY 失败: %v", err))
		return fmt.Errorf("request pty: %w", err)
	}

	// 请求 Shell
	if err := session.Shell(); err != nil {
		w.closeConnections()
		w.sendStatus(event.StatusError, fmt.Sprintf("请求 Shell 失败: %v", err))
		return fmt.Errorf("request shell: %w", err)
	}

	// 启动读取 goroutine（SSH stdout → uiChan）
	w.wg.Add(1)
	go w.readLoop(stdout)

	// 启动输入 goroutine（inputChan → SSH stdin）
	w.wg.Add(1)
	go w.inputLoop()

	// 启动配置的隧道
	for _, tunnel := range w.session.Tunnels {
		if err := w.startTunnel(tunnel); err != nil {
			log.Warn("failed to start tunnel", "type", tunnel.Type, "err", err)
		}
	}

	// 发送已连接状态
	w.sendStatus(event.StatusConnected, fmt.Sprintf("已连接 %s@%s:%d", w.session.Username, w.session.Host, w.session.Port))
	log.Info("ssh session started", "host", w.session.Host, "user", w.session.Username)

	return nil
}

// startTunnel 根据配置启动隧道
func (w *Worker) startTunnel(tunnel config.TunnelConfig) error {
	switch tunnel.Type {
	case "local":
		return w.LocalForward(tunnel.LocalAddr, tunnel.RemoteAddr)
	case "remote":
		return w.RemoteForward(tunnel.RemoteAddr, tunnel.LocalAddr)
	case "dynamic":
		return w.DynamicForward(tunnel.LocalAddr)
	default:
		return fmt.Errorf("unknown tunnel type: %s", tunnel.Type)
	}
}

// readLoop 从 SSH stdout 读取数据并发送到 UI。
// 当 SSH 连接断开时发送 StatusDisconnected 事件。
func (w *Worker) readLoop(stdout io.Reader) {
	defer w.wg.Done()

	buf := make([]byte, 8192)
	for {
		n, err := stdout.Read(buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			select {
			case w.uiChan <- event.UIEvent{
				TabID:        w.session.ID,
				Type:         event.EventTerminal,
				TerminalData: data,
			}:
			case <-w.ctx.Done():
				return
			}
		}
		if err != nil {
			break
		}
	}

	// 等待会话完全结束
	w.mu.Lock()
	session := w.sshSession
	w.mu.Unlock()
	if session != nil {
		session.Wait()
	}

	// 取消 context 以停止 inputLoop 和其他 goroutine
	if w.cancel != nil {
		w.cancel()
	}

	// 更新连接状态
	w.mu.Lock()
	w.connected = false
	w.mu.Unlock()

	// 发送断开连接状态
	w.sendStatus(event.StatusDisconnected, "连接已断开")
	log.Info("ssh session ended", "host", w.session.Host)
}

// inputLoop 从 inputChan 读取键盘输入并发送到 SSH stdin
func (w *Worker) inputLoop() {
	defer w.wg.Done()

	for {
		select {
		case data := <-w.inputChan:
			if err := w.Write(data); err != nil {
				log.Warn("write to ssh stdin failed", "err", err)
			}
		case <-w.ctx.Done():
			return
		}
	}
}

// Write 发送数据到 SSH stdin。
// 此方法可直接调用（如快捷命令），也可通过 inputChan 间接调用。
func (w *Worker) Write(data []byte) error {
	w.mu.Lock()
	stdin := w.stdin
	w.mu.Unlock()

	if stdin == nil {
		return fmt.Errorf("not connected")
	}

	_, err := stdin.Write(data)
	return err
}

// SendInput 通过 inputChan 发送键盘输入。
// UI 层调用此方法将键盘事件转发到 SSH。
func (w *Worker) SendInput(data []byte) {
	select {
	case w.inputChan <- data:
	case <-w.ctx.Done():
	}
}

// Resize 调整 PTY 窗口大小
func (w *Worker) Resize(cols, rows int) error {
	w.mu.Lock()
	w.cols = cols
	w.rows = rows
	session := w.sshSession
	w.mu.Unlock()

	if session == nil {
		return fmt.Errorf("session not started")
	}

	return session.WindowChange(rows, cols)
}

// Close 关闭连接并清理所有资源（goroutine、连接、隧道）。
// 此方法是幂等的，可安全多次调用。
func (w *Worker) Close() {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.closed = true
	w.mu.Unlock()

	// 停止隧道
	w.StopTunnels()

	// 取消 context，通知 goroutine 退出
	if w.cancel != nil {
		w.cancel()
	}

	// 关闭连接（会解除 goroutine 的阻塞）
	w.closeConnections()

	// 等待所有 goroutine 结束
	w.wg.Wait()

	log.Info("ssh worker closed", "host", w.session.Host)
}

// closeConnections 关闭 SSH 连接和会话。
// 在锁外执行关闭操作以避免死锁。
func (w *Worker) closeConnections() {
	w.mu.Lock()
	session := w.sshSession
	stdin := w.stdin
	client := w.client
	w.sshSession = nil
	w.stdin = nil
	w.client = nil
	w.connected = false
	w.mu.Unlock()

	// 在锁外关闭资源，避免与 Write 等方法死锁
	if stdin != nil {
		stdin.Close()
	}
	if session != nil {
		session.Close()
	}
	if client != nil {
		client.Close()
	}
}

// IsConnected 返回当前是否已连接
func (w *Worker) IsConnected() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.connected
}

// Client 返回底层 SSH 客户端（供 SFTP 等模块使用）
func (w *Worker) Client() *ssh.Client {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.client
}

// SessionID 返回会话 ID
func (w *Worker) SessionID() string {
	return w.session.ID
}

// sendStatus 发送连接状态事件到 UI（非阻塞）
func (w *Worker) sendStatus(status event.ConnectionStatus, msg string) {
	select {
	case w.uiChan <- event.UIEvent{
		TabID:     w.session.ID,
		Type:      event.EventStatus,
		Status:    status,
		StatusMsg: msg,
	}:
	default:
		// UI 通道已满，丢弃事件以避免阻塞
	}
}
