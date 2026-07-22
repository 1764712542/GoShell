// Package localterminal 通过 PTY 运行系统 shell，提供本地终端会话。
// 接口设计与 ssh/serial/telnet Worker 保持一致，便于在 Tab 中统一管理。
package localterminal

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sync"

	"github.com/creack/pty"

	"github.com/zhuyao/meatshell/internal/event"
	"github.com/zhuyao/meatshell/internal/log"
)

// Worker 管理本地终端会话，通过 PTY 运行系统 shell。
type Worker struct {
	shell    string
	uiChan   chan event.UIEvent
	tabID    string

	cmd    *exec.Cmd
	pty    *os.File

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu        sync.Mutex
	connected bool
	closed    bool
	cols      int
	rows      int
}

// NewWorker 创建本地终端 worker。
// shell 指定要运行的 shell（如 /bin/bash, /bin/zsh, powershell.exe）。
// 如果 shell 为空，自动检测系统默认 shell。
func NewWorker(shell string, uiChan chan event.UIEvent, tabID string) *Worker {
	if shell == "" {
		shell = detectDefaultShell()
	}
	return &Worker{
		shell:  shell,
		uiChan: uiChan,
		tabID:  tabID,
		cols:   80,
		rows:   24,
	}
}

// detectDefaultShell 根据操作系统自动检测默认 shell。
func detectDefaultShell() string {
	switch runtime.GOOS {
	case "windows":
		// Windows 优先使用 PowerShell，回退到 cmd
		if path, err := exec.LookPath("powershell.exe"); err == nil {
			return path
		}
		if path, err := exec.LookPath("cmd.exe"); err == nil {
			return path
		}
		return "powershell.exe"
	default:
		// macOS/Linux：优先使用 $SHELL 环境变量，默认 /bin/bash
		if shell := os.Getenv("SHELL"); shell != "" {
			return shell
		}
		if path, err := exec.LookPath("zsh"); err == nil {
			return path
		}
		if path, err := exec.LookPath("bash"); err == nil {
			return path
		}
		return "/bin/bash"
	}
}

// Connect 启动本地 PTY 会话。
// 流程：
//  1. 创建 exec.Command 运行指定 shell
//  2. 通过 creack/pty 创建 PTY
//  3. 启动读取 goroutine，将 PTY 输出转发到 uiChan
//  4. 启动输入 goroutine，将 inputChan 数据写入 PTY
func (w *Worker) Connect(ctx context.Context) error {
	w.mu.Lock()
	if w.connected {
		w.mu.Unlock()
		return fmt.Errorf("already connected")
	}
	w.mu.Unlock()

	// 发送连接中状态
	w.sendStatus(event.StatusConnecting, fmt.Sprintf("正在启动本地终端 %s...", w.shell))

	// 创建可取消的 context
	w.ctx, w.cancel = context.WithCancel(ctx)

	// 创建 exec.Command 运行 shell
	cmd := exec.CommandContext(w.ctx, w.shell)
	cmd.Env = os.Environ()

	// 通过 creack/pty 启动 PTY
	ptyFile, err := pty.StartWithSize(cmd, &pty.Winsize{
		Cols: uint16(w.cols),
		Rows: uint16(w.rows),
	})
	if err != nil {
		w.sendStatus(event.StatusError, fmt.Sprintf("启动本地终端失败: %v", err))
		return fmt.Errorf("local pty start: %w", err)
	}

	w.mu.Lock()
	w.cmd = cmd
	w.pty = ptyFile
	w.connected = true
	w.mu.Unlock()

	// 启动读取 goroutine（PTY 输出 → uiChan）
	w.wg.Add(1)
	go w.readLoop()

	// 发送已连接状态
	w.sendStatus(event.StatusConnected, fmt.Sprintf("本地终端就绪 (%s)", w.shell))
	log.Info("local terminal started", "shell", w.shell)

	return nil
}

// readLoop 从 PTY 读取数据并发送到 UI。
// 当 PTY 关闭或 context 取消时退出。
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
		ptyFile := w.pty
		w.mu.Unlock()

		if ptyFile == nil {
			break
		}

		n, err := ptyFile.Read(buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			select {
			case w.uiChan <- event.UIEvent{
				TabID:        w.tabID,
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

	// 更新连接状态
	w.mu.Lock()
	w.connected = false
	w.mu.Unlock()

	w.sendStatus(event.StatusDisconnected, "本地终端已退出")
	log.Info("local terminal ended", "shell", w.shell)
}

// SendInput 向 PTY 写入用户输入。
// UI 层调用此方法将键盘事件转发到本地 shell。
func (w *Worker) SendInput(data []byte) {
	w.mu.Lock()
	ptyFile := w.pty
	w.mu.Unlock()

	if ptyFile == nil {
		return
	}

	if _, err := ptyFile.Write(data); err != nil {
		log.Warn("local pty write failed", "err", err)
	}
}

// Close 关闭本地终端并清理所有资源。
// 此方法是幂等的，可安全多次调用。
func (w *Worker) Close() {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.closed = true
	cmd := w.cmd
	ptyFile := w.pty
	w.mu.Unlock()

	// 取消 context，通知 goroutine 退出
	if w.cancel != nil {
		w.cancel()
	}

	// 关闭 PTY（会解除 readLoop 的阻塞）
	if ptyFile != nil {
		ptyFile.Close()
	}

	// 等待 shell 进程退出
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}

	// 等待读取 goroutine 结束
	w.wg.Wait()

	log.Info("local terminal worker closed", "shell", w.shell)
}

// IsConnected 返回当前是否已连接
func (w *Worker) IsConnected() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.connected
}

// Resize 调整 PTY 窗口大小
func (w *Worker) Resize(cols, rows int) error {
	w.mu.Lock()
	w.cols = cols
	w.rows = rows
	ptyFile := w.pty
	w.mu.Unlock()

	if ptyFile == nil {
		return fmt.Errorf("pty not started")
	}

	return pty.Setsize(ptyFile, &pty.Winsize{
		Cols: uint16(cols),
		Rows: uint16(rows),
	})
}

// sendStatus 发送连接状态事件到 UI（非阻塞）
func (w *Worker) sendStatus(status event.ConnectionStatus, msg string) {
	select {
	case w.uiChan <- event.UIEvent{
		TabID:     w.tabID,
		Type:      event.EventStatus,
		Status:    status,
		StatusMsg: msg,
	}:
	default:
		// UI 通道已满，丢弃事件以避免阻塞
	}
}
