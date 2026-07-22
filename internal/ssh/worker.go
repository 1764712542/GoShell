package ssh

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

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

	// Keepalive 相关
	keepaliveTicker    *time.Ticker
	keepaliveInterval  time.Duration
	keepaliveCount     int
	maxKeepaliveFailures int

	// 主机密钥确认回调
	hostKeyConfirm chan bool

	// 活跃的隧道列表
	tunnels []*tunnelInfo
}

// NewWorker 创建一个新的 SSH Worker。
// uiChan 用于向 UI 发送事件，inputChan 用于接收键盘输入。
func NewWorker(sess *config.Session, uiChan chan event.UIEvent) *Worker {
	return &Worker{
		session:            sess,
		uiChan:             uiChan,
		inputChan:          make(chan []byte, 256),
		hostKeyConfirm:     make(chan bool, 1),
		keepaliveInterval:  30 * time.Second,
		maxKeepaliveFailures: 3,
		cols:               80,
		rows:               24,
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

	// 启动 keepalive goroutine
	w.wg.Add(1)
	go w.keepaliveLoop()

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

// keepaliveLoop 定期发送 keepalive 请求以保持 SSH 连接活跃。
// 连续失败超过 maxKeepaliveFailures 次后退出，连接将被视为已断开。
func (w *Worker) keepaliveLoop() {
	defer w.wg.Done()
	ticker := time.NewTicker(w.keepaliveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			// Send keepalive request
			_, _, err := w.client.SendRequest("keepalive@openssh.com", true, nil)
			if err != nil {
				w.mu.Lock()
				w.keepaliveCount++
				exceeded := w.keepaliveCount >= w.maxKeepaliveFailures
				w.mu.Unlock()
				if exceeded {
					log.Warn("keepalive failed, connection may be dead", "host", w.session.Host)
					return
				}
			} else {
				w.mu.Lock()
				w.keepaliveCount = 0
				w.mu.Unlock()
			}
		case <-w.ctx.Done():
			return
		}
	}
}

// dialViaJumpHost 通过跳板机（ProxyJump）建立到目标主机的 SSH 连接。
// 流程：
//  1. 解析 ProxyJump 值（格式: [user@]host[:port]）
//  2. 连接跳板机（使用与目标主机相同的认证方式）
//  3. 通过跳板机的 SSH 连接拨号到目标主机
//  4. 在隧道连接上完成 SSH 握手
func (w *Worker) dialViaJumpHost(ctx context.Context) (*ssh.Client, error) {
	// 解析 ProxyJump 值: [user@]host[:port]
	jumpUser, jumpHost, jumpPort, err := parseProxyJump(w.session.ProxyJump, w.session.Username)
	if err != nil {
		return nil, fmt.Errorf("parse proxy jump: %w", err)
	}

	// 构建目标主机的 SSH 客户端配置
	targetSSHConfig, err := w.buildSSHConfig()
	if err != nil {
		return nil, err
	}

	// 构建跳板机的 SSH 客户端配置（复用相同的认证方式）
	methods, err := w.authMethods()
	if err != nil {
		return nil, fmt.Errorf("build jump host auth methods: %w", err)
	}
	jumpSSHConfig := &ssh.ClientConfig{
		User:            jumpUser,
		HostKeyCallback: w.hostKeyCallback,
		Timeout:         30 * time.Second,
		Config:          targetSSHConfig.Config,
		Auth:            methods,
	}

	jumpTarget := fmt.Sprintf("%s:%d", jumpHost, jumpPort)
	targetAddr := fmt.Sprintf("%s:%d", w.session.Host, w.session.Port)

	log.Info("connecting to jump host", "jump_host", jumpHost, "jump_port", jumpPort, "target", targetAddr)

	// 连接跳板机（通过代理或直连）
	jumpConn, err := dialViaProxy(w.session.Proxy, jumpTarget)
	if err != nil {
		return nil, fmt.Errorf("dial jump host: %w", err)
	}

	// 检查 context 是否已取消
	select {
	case <-ctx.Done():
		jumpConn.Close()
		return nil, ctx.Err()
	default:
	}

	// 跳板机 SSH 握手
	jumpSSHConn, jumpChans, jumpReqs, err := ssh.NewClientConn(jumpConn, jumpTarget, jumpSSHConfig)
	if err != nil {
		jumpConn.Close()
		return nil, fmt.Errorf("ssh connect jump host: %w", err)
	}
	jumpClient := ssh.NewClient(jumpSSHConn, jumpChans, jumpReqs)
	defer jumpClient.Close()

	log.Info("connected to jump host, dialing target", "target", targetAddr)

	// 通过跳板机拨号到目标主机
	conn, err := jumpClient.Dial("tcp", targetAddr)
	if err != nil {
		return nil, fmt.Errorf("dial target via jump host: %w", err)
	}

	// 检查 context 是否已取消
	select {
	case <-ctx.Done():
		conn.Close()
		return nil, ctx.Err()
	default:
	}

	// 在隧道连接上完成 SSH 握手
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, targetAddr, targetSSHConfig)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("ssh connect via jump: %w", err)
	}

	client := ssh.NewClient(sshConn, chans, reqs)
	log.Info("ssh connected via jump host", "host", w.session.Host, "jump", jumpHost)

	return client, nil
}

// parseProxyJump 解析 ProxyJump 值，格式为 [user@]host[:port]。
// 如果未指定 user，使用 defaultUser；如果未指定 port，使用 22。
func parseProxyJump(proxyJump, defaultUser string) (user, host string, port int, err error) {
	port = 22
	s := proxyJump

	// 提取 user
	if idx := strings.Index(s, "@"); idx >= 0 {
		user = s[:idx]
		s = s[idx+1:]
	} else {
		user = defaultUser
	}

	// 提取 port（最后一个冒号之后的部分）
	if idx := strings.LastIndex(s, ":"); idx >= 0 {
		if p, parseErr := strconv.Atoi(s[idx+1:]); parseErr == nil {
			port = p
			s = s[:idx]
		}
	}

	host = s
	if host == "" {
		return "", "", 0, fmt.Errorf("invalid proxy jump format: %s", proxyJump)
	}
	return user, host, port, nil
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
