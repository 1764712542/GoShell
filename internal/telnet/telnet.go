package telnet

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/zhuyao/meatshell/internal/event"
	"github.com/zhuyao/meatshell/internal/log"
)

// Telnet 协议控制字符
const (
	IAC  = 0xFF // Interpret As Command
	DONT = 0xFE
	DO   = 0xFD
	WONT = 0xFC
	WILL = 0xFB
	SB   = 0xFA // Subnegotiation Begin
	SE   = 0xF0 // Subnegotiation End
)

// Worker 管理一个 Telnet 会话的生命周期，包括连接建立、
// 数据读写和基本的 Telnet 协议处理。接口设计与 ssh.Worker 保持一致。
type Worker struct {
	host   string
	port   int
	uiChan chan event.UIEvent
	tabID  string

	conn   net.Conn
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu        sync.Mutex
	connected bool
	closed    bool
}

// NewWorker 创建一个新的 Telnet Worker。
// host 是目标主机地址，port 是目标端口（通常为 23）。
func NewWorker(host string, port int, uiChan chan event.UIEvent, tabID string) *Worker {
	return &Worker{
		host:   host,
		port:   port,
		uiChan: uiChan,
		tabID:  tabID,
	}
}

// Connect 建立 TCP 连接并启动读取循环。
// 流程：
//  1. 通过 net.Dial 建立 TCP 连接
//  2. 创建可取消的 context
//  3. 启动读取 goroutine，过滤 Telnet 协议命令后将数据转发到 uiChan
func (w *Worker) Connect(ctx context.Context) error {
	w.mu.Lock()
	if w.connected {
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
		return fmt.Errorf("telnet dial %s: %w", address, err)
	}

	w.mu.Lock()
	w.conn = conn
	w.connected = true
	w.ctx, w.cancel = context.WithCancel(ctx)
	w.mu.Unlock()

	// 启动读取 goroutine
	w.wg.Add(1)
	go w.readLoop()

	w.sendStatus(event.StatusConnected, fmt.Sprintf("已连接 %s:%d", w.host, w.port))
	log.Info("telnet connected", "host", w.host, "port", w.port)

	return nil
}

// readLoop 从 Telnet 连接读取数据，过滤 IAC 命令后发送到 UI。
// Telnet 协议命令（以 0xFF 开头的序列）会被过滤，只保留有效数据。
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
			// 过滤 Telnet 协议命令
			data := filterTelnetCommands(buf[:n])
			if len(data) > 0 {
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
		}
		if err != nil {
			// 检查是否为超时
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// 读取超时，继续循环
				continue
			}
			// 其他错误，退出读取循环
			if err.Error() != "EOF" {
				log.Warn("telnet read error", "err", err)
			}
			break
		}
	}

	// 更新连接状态
	w.mu.Lock()
	w.connected = false
	w.mu.Unlock()

	w.sendStatus(event.StatusDisconnected, "连接已断开")
	log.Info("telnet session ended", "host", w.host)
}

// SendInput 向 Telnet 连接写入数据。
// UI 层调用此方法将用户输入转发到远程服务器。
func (w *Worker) SendInput(data []byte) {
	w.mu.Lock()
	conn := w.conn
	w.mu.Unlock()

	if conn == nil {
		return
	}

	if _, err := conn.Write(data); err != nil {
		log.Warn("telnet write failed", "err", err)
	}
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

	// 等待读取 goroutine 结束
	w.wg.Wait()

	log.Info("telnet worker closed", "host", w.host)
}

// IsConnected 返回当前是否已连接
func (w *Worker) IsConnected() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.connected
}


// Resize 调整终端窗口大小。
// Telnet 协议不支持窗口大小变更，此方法无操作。
func (w *Worker) Resize(cols, rows int) error {
	// no-op: Telnet protocol does not support window resize
	return nil
}

// SessionID 返回会话 ID
func (w *Worker) SessionID() string {
	return w.tabID
}

// sendStatus 发送连接状态事件到 UI（非阻塞）
// sendStatus 发送连接状态事件到 UI（阻塞 + 超时，确保关键状态事件不被丢弃）
func (w *Worker) sendStatus(status event.ConnectionStatus, msg string) {
	evt := event.UIEvent{
		TabID:     w.tabID,
		Type:      event.EventStatus,
		Status:    status,
		StatusMsg: msg,
	}
	var done <-chan struct{}
	if w.ctx != nil {
		done = w.ctx.Done()
	}
	select {
	case w.uiChan <- evt:
	case <-done:
	case <-time.After(2 * time.Second):
		log.Warn("sendStatus timed out, UI may be unresponsive", "status", status, "msg", msg)
	}
}

// filterTelnetCommands 过滤 Telnet 协议命令（IAC 序列），
// 只保留有效数据。这是一个简化的实现，会拒绝所有协商请求。
func filterTelnetCommands(data []byte) []byte {
	var result bytes.Buffer
	i := 0
	for i < len(data) {
		if data[i] == IAC {
			// 处理 IAC 命令序列
			if i+1 >= len(data) {
				// 数据不完整，跳过
				break
			}
			cmd := data[i+1]
			switch cmd {
			case DO, DONT, WILL, WONT:
				// 双字节命令：IAC + cmd + option
				if i+2 < len(data) {
					// 简化处理：拒绝所有 DO/WILL 请求
					i += 3
				} else {
					i = len(data)
				}
			case SB:
				// 子协商：IAC SB ... IAC SE
				// 跳过到 IAC SE
				i += 2
				for i < len(data)-1 {
					if data[i] == IAC && data[i+1] == SE {
						i += 2
						break
					}
					i++
				}
				if i >= len(data)-1 {
					i = len(data)
				}
			default:
				// 其他命令：IAC + cmd
				i += 2
			}
		} else {
			// 普通数据
			result.WriteByte(data[i])
			i++
		}
	}
	return result.Bytes()
}
