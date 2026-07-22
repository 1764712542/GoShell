// Package mosh 通过外部 mosh 客户端提供 Mosh (Mobile Shell) 会话。
// Mosh 通过 SSH 协商后切换到 UDP，提供更好的网络漫游和断线恢复能力。
// 本包通过 PTY 运行外部 mosh 命令实现，避免重新实现复杂的 Mosh 协议。
package mosh

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/creack/pty"

	"github.com/zhuyao/meatshell/internal/config"
	"github.com/zhuyao/meatshell/internal/event"
	"github.com/zhuyao/meatshell/internal/log"
)

// Worker 管理一个 Mosh 会话的完整生命周期。
// 通过 PTY 运行外部 mosh 客户端二进制文件，将 PTY 输入输出转发到 UI。
type Worker struct {
	session   *config.Session
	uiChan    chan event.UIEvent // 发送给 UI 的事件通道
	inputChan chan []byte        // 接收键盘输入的通道

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.Reader

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu        sync.Mutex
	connected bool
	closed    bool
	cols      int
	rows      int
}

// NewWorker 创建一个新的 Mosh Worker。
// uiChan 用于向 UI 发送事件，inputChan 在内部创建用于接收键盘输入。
func NewWorker(sess *config.Session, uiChan chan event.UIEvent) *Worker {
	return &Worker{
		session:   sess,
		uiChan:    uiChan,
		inputChan: make(chan []byte, 256),
		cols:      80,
		rows:      24,
	}
}

// Connect 建立 Mosh 会话。
// 流程：
//  1. 检查 mosh 二进制是否存在，给出友好的错误提示
//  2. 构造 mosh 命令参数（用户、主机、SSH 端口）
//  3. 通过 creack/pty 启动 PTY
//  4. 启动读取和输入 goroutine
func (w *Worker) Connect(ctx context.Context) error {
	w.mu.Lock()
	if w.connected {
		w.mu.Unlock()
		return fmt.Errorf("already connected")
	}
	w.mu.Unlock()

	// 检查 mosh 二进制是否存在
	moshPath, err := exec.LookPath("mosh")
	if err != nil {
		return fmt.Errorf("mosh binary not found: %w\n"+
			"请安装 mosh 后重试：\n"+
			"  macOS:  brew install mosh\n"+
			"  Ubuntu: sudo apt install mosh\n"+
			"  Fedora: sudo dnf install mosh", err)
	}

	// 创建可取消的 context
	w.ctx, w.cancel = context.WithCancel(ctx)

	// 发送连接中状态
	w.sendStatus(event.StatusConnecting, fmt.Sprintf("正在通过 mosh 连接 %s:%d...", w.session.Host, w.session.Port))

	// 构造 mosh 命令参数
	args := w.buildMoshArgs()
	cmd := exec.CommandContext(w.ctx, moshPath, args...)

	// 设置终端类型环境变量
	termType := w.session.TermType
	if termType == "" {
		termType = "xterm-256color"
	}
	cmd.Env = append(os.Environ(), "TERM="+termType)

	// 通过 creack/pty 启动 PTY
	w.mu.Lock()
	cols, rows := w.cols, w.rows
	w.mu.Unlock()

	ptyFile, err := pty.StartWithSize(cmd, &pty.Winsize{
		Cols: uint16(cols),
		Rows: uint16(rows),
	})
	if err != nil {
		w.sendStatus(event.StatusError, fmt.Sprintf("启动 mosh 失败: %v", err))
		return fmt.Errorf("mosh pty start: %w", err)
	}

	w.mu.Lock()
	w.cmd = cmd
	w.stdin = ptyFile
	w.stdout = ptyFile
	w.connected = true
	w.mu.Unlock()

	// 启动读取 goroutine（mosh stdout → uiChan）
	w.wg.Add(1)
	go w.readLoop()

	// 启动输入 goroutine（inputChan → mosh stdin）
	w.wg.Add(1)
	go w.inputLoop()

	// 发送已连接状态
	w.sendStatus(event.StatusConnected, fmt.Sprintf("已连接 %s@%s:%d (mosh)", w.session.Username, w.session.Host, w.session.Port))
	log.Info("mosh session started", "host", w.session.Host, "user", w.session.Username)

	return nil
}

// buildMoshArgs 构造 mosh 命令行参数。
// 通过 --ssh 选项传递 SSH 端口，mosh 会使用该 ssh 命令进行初始协商。
func (w *Worker) buildMoshArgs() []string {
	var args []string

	// 指定 SSH 端口（mosh 默认使用 22 端口）
	if w.session.Port != 0 && w.session.Port != 22 {
		args = append(args, fmt.Sprintf("--ssh=ssh -p %d", w.session.Port))
	}

	// 构造 user@host
	target := w.session.Host
	if w.session.Username != "" {
		target = w.session.Username + "@" + w.session.Host
	}
	args = append(args, target)

	return args
}

// readLoop 从 mosh stdout 读取数据并发送到 UI。
// 当 mosh 进程退出或 context 取消时发送 StatusDisconnected 事件。
func (w *Worker) readLoop() {
	defer w.wg.Done()

	buf := make([]byte, 8192)
	for {
		// 检查 context 是否已取消
		select {
		case <-w.ctx.Done():
			return
		default:
		}

		w.mu.Lock()
		stdout := w.stdout
		w.mu.Unlock()

		if stdout == nil {
			break
		}

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

	// 取消 context 以停止 inputLoop 和其他 goroutine
	if w.cancel != nil {
		w.cancel()
	}

	// 更新连接状态
	w.mu.Lock()
	w.connected = false
	w.mu.Unlock()

	// 发送断开连接状态
	w.sendStatus(event.StatusDisconnected, "mosh 连接已断开")
	log.Info("mosh session ended", "host", w.session.Host)
}

// inputLoop 从 inputChan 读取键盘输入并写入 mosh stdin。
func (w *Worker) inputLoop() {
	defer w.wg.Done()

	for {
		select {
		case data := <-w.inputChan:
			if err := w.write(data); err != nil {
				log.Warn("write to mosh stdin failed", "err", err)
			}
		case <-w.ctx.Done():
			return
		}
	}
}

// write 直接写入 mosh stdin（内部方法）。
func (w *Worker) write(data []byte) error {
	w.mu.Lock()
	stdin := w.stdin
	w.mu.Unlock()

	if stdin == nil {
		return fmt.Errorf("not connected")
	}

	_, err := stdin.Write(data)
	return err
}

// SendInput 通过 inputChan 异步发送键盘输入。
// UI 层调用此方法将键盘事件转发到 mosh。
func (w *Worker) SendInput(data []byte) {
	select {
	case w.inputChan <- data:
	case <-w.ctx.Done():
	}
}

// Resize 调整 PTY 窗口大小。
func (w *Worker) Resize(cols, rows int) error {
	w.mu.Lock()
	w.cols = cols
	w.rows = rows
	stdout := w.stdout
	w.mu.Unlock()

	if stdout == nil {
		return fmt.Errorf("mosh session not started")
	}

	// ptyFile 同时作为 stdout，通过类型断言获取 *os.File
	ptyFile, ok := stdout.(*os.File)
	if !ok {
		return fmt.Errorf("unsupported pty type")
	}

	return pty.Setsize(ptyFile, &pty.Winsize{
		Cols: uint16(cols),
		Rows: uint16(rows),
	})
}

// Close 关闭 mosh 会话并清理所有资源。
// 此方法是幂等的，可安全多次调用。
func (w *Worker) Close() {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.closed = true
	cmd := w.cmd
	stdin := w.stdin
	w.mu.Unlock()

	// 取消 context，通知 goroutine 退出
	if w.cancel != nil {
		w.cancel()
	}

	// 关闭 PTY（会解除 readLoop 的阻塞）
	if stdin != nil {
		stdin.Close()
	}

	// 终止 mosh 进程
	if cmd != nil && cmd.Process != nil {
		// 发送 SIGTERM 让 mosh 优雅退出
		_ = cmd.Process.Signal(syscall.SIGTERM)
		// 等待进程退出
		_, _ = cmd.Process.Wait()
	}

	// 等待所有 goroutine 结束
	w.wg.Wait()

	log.Info("mosh worker closed", "host", w.session.Host)
}

// IsConnected 返回当前是否已连接
func (w *Worker) IsConnected() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.connected
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
