// Package rlogin 实现了 RLogin 协议（RFC 1282）的客户端，用于通过
// RLogin 连接远程主机并交互式地收发终端数据。
package rlogin

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/zhuyao/meatshell/internal/event"
	"github.com/zhuyao/meatshell/internal/log"
)

// RLogin 协议常量
const (
	defaultPort = 513
	termType    = "xterm-256color"
	termSpeed   = "9600"
)

// Worker 管理一个 RLogin (RFC 1282) 会话的生命周期，包括连接建立、
// 协议握手、数据读写和连接清理。接口设计与 telnet.Worker 保持一致。
type Worker struct {
	host   string
	port   int
	uiChan chan event.UIEvent
	tabID  string

	conn      net.Conn
	inputChan chan []byte

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu     sync.Mutex
	closed bool
}

// NewWorker 创建一个新的 RLogin Worker。
// host 是目标主机地址，port 是目标端口（通常为 513）。
// 如果 port 为 0，则使用 RLogin 默认端口 513。
func NewWorker(host string, port int, uiChan chan event.UIEvent, tabID string) *Worker {
	if port == 0 {
		port = defaultPort
	}
	return &Worker{
		host:      host,
		port:      port,
		uiChan:    uiChan,
		tabID:     tabID,
		inputChan: make(chan []byte, 256),
	}
}

// Connect 建立 TCP 连接并完成 RLogin 协议握手。
// 流程：
//  1. 通过 net.Dial 建立 TCP 连接
//  2. 发送 RLogin 握手：\x00user\x00term\x00speed\x00
//  3. 读取服务器响应（单个 0x00 字节表示成功）
//  4. 创建可取消的 context
//  5. 启动读取和输入 goroutine
func (w *Worker) Connect(ctx context.Context) error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return fmt.Errorf("worker is closed")
	}
	if w.conn != nil {
		w.mu.Unlock()
		return fmt.Errorf("already connected")
	}
	w.mu.Unlock()

	// 发送连接中状态
	w.sendStatus(event.StatusConnecting, fmt.Sprintf("正在连接 %s:%d...", w.host, w.port))

	// 建立 TCP 连接
	address := net.JoinHostPort(w.host, fmt.Sprintf("%d", w.port))
	conn, err := net.DialTimeout("tcp", address, 10*time.Second)
	if err != nil {
		w.sendStatus(event.StatusError, fmt.Sprintf("连接失败: %v", err))
		return fmt.Errorf("rlogin dial %s: %w", address, err)
	}

	// 发送 RLogin 协议握手
	// 客户端发送：\x00user\x00term\x00speed\x00
	// 由于 Worker 不携带用户名，此处使用空字符串作为 client-user。
	user := ""
	handshake := fmt.Sprintf("\x00%s\x00%s\x00%s\x00", user, termType, termSpeed)
	if _, err := conn.Write([]byte(handshake)); err != nil {
		conn.Close()
		w.sendStatus(event.StatusError, fmt.Sprintf("握手发送失败: %v", err))
		return fmt.Errorf("rlogin handshake write: %w", err)
	}

	// 读取服务器响应（单个 0x00 字节表示成功，0x01 表示拒绝）
	resp := make([]byte, 1)
	if _, err := io.ReadFull(conn, resp); err != nil {
		conn.Close()
		w.sendStatus(event.StatusError, fmt.Sprintf("握手响应读取失败: %v", err))
		return fmt.Errorf("rlogin handshake read: %w", err)
	}
	if resp[0] != 0x00 {
		conn.Close()
		w.sendStatus(event.StatusError, fmt.Sprintf("服务器拒绝连接: 0x%02x", resp[0]))
		return fmt.Errorf("rlogin handshake rejected: 0x%02x", resp[0])
	}

	w.mu.Lock()
	w.conn = conn
	w.ctx, w.cancel = context.WithCancel(ctx)
	w.mu.Unlock()

	// 启动读取 goroutine（conn → uiChan）
	w.wg.Add(1)
	go w.readLoop()

	// 启动输入 goroutine（inputChan → conn）
	w.wg.Add(1)
	go w.inputLoop()

	w.sendStatus(event.StatusConnected, fmt.Sprintf("已连接 %s:%d", w.host, w.port))
	log.Info("rlogin connected", "host", w.host, "port", w.port)

	return nil
}

// readLoop 从 RLogin 连接读取数据并发送到 UI。
// 当连接断开时发送 StatusDisconnected 事件并通知 inputLoop 退出。
func (w *Worker) readLoop() {
	defer w.wg.Done()

	buf := make([]byte, 4096)
	for {
		// 检查 context 是否已取消
		select {
		case <-w.ctx.Done():
			return
		default:
		}

		w.mu.Lock()
		conn := w.conn
		w.mu.Unlock()

		if conn == nil {
			break
		}

		// 设置读取截止时间，以便定期检查 context
		conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

		n, err := conn.Read(buf)
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
			// 检查是否为超时
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// 读取超时，继续循环
				continue
			}
			// 其他错误，退出读取循环
			if err.Error() != "EOF" {
				log.Warn("rlogin read error", "err", err)
			}
			break
		}
	}

	// 取消 context 以停止 inputLoop
	if w.cancel != nil {
		w.cancel()
	}

	w.sendStatus(event.StatusDisconnected, "连接已断开")
	log.Info("rlogin session ended", "host", w.host)
}

// inputLoop 从 inputChan 读取键盘输入并写入 RLogin 连接。
func (w *Worker) inputLoop() {
	defer w.wg.Done()

	for {
		select {
		case data := <-w.inputChan:
			w.mu.Lock()
			conn := w.conn
			w.mu.Unlock()

			if conn == nil {
				return
			}

			if _, err := conn.Write(data); err != nil {
				log.Warn("rlogin write failed", "err", err)
			}
		case <-w.ctx.Done():
			return
		}
	}
}

// SendInput 通过 inputChan 非阻塞地发送键盘输入。
// UI 层调用此方法将键盘事件转发到远程服务器。
// 如果输入通道已满，数据将被丢弃以避免阻塞 UI。
func (w *Worker) SendInput(data []byte) {
	select {
	case w.inputChan <- data:
	default:
		// 输入通道已满，丢弃数据以避免阻塞 UI
	}
}

// Resize 调整终端窗口大小。
// RLogin 协议不支持窗口大小变更，此方法无操作。
func (w *Worker) Resize(cols, rows int) error {
	return nil
}

// Close 关闭连接并清理所有资源。
// 此方法是幂等的，可安全多次调用。
func (w *Worker) Close() {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.closed = true
	conn := w.conn
	w.mu.Unlock()

	// 取消 context，通知 goroutine 退出
	if w.cancel != nil {
		w.cancel()
	}

	// 关闭连接（会解除 readLoop 的阻塞）
	if conn != nil {
		conn.Close()
	}

	// 等待所有 goroutine 结束
	w.wg.Wait()

	log.Info("rlogin worker closed", "host", w.host)
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
