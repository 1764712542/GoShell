package serial

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.bug.st/serial"

	"github.com/zhuyao/meatshell/internal/event"
	"github.com/zhuyao/meatshell/internal/log"
)

// Worker 管理一个串口会话的生命周期，包括连接建立、
// 数据读写和状态通知。接口设计与 ssh.Worker 保持一致。
type Worker struct {
	portName string
	baudRate int
	uiChan   chan event.UIEvent
	tabID    string

	port   serial.Port
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu        sync.Mutex
	connected bool
	closed    bool
}

// NewWorker 创建一个新的串口 Worker。
// portName 是串口设备路径（如 /dev/ttyUSB0、COM3），
// baudRate 是波特率（如 9600、115200）。
func NewWorker(portName string, baudRate int, uiChan chan event.UIEvent, tabID string) *Worker {
	return &Worker{
		portName: portName,
		baudRate: baudRate,
		uiChan:   uiChan,
		tabID:    tabID,
	}
}

// Connect 打开串口并启动读取循环。
// 流程：
//  1. 打开串口设备
//  2. 创建可取消的 context
//  3. 启动读取 goroutine，将串口数据转发到 uiChan
func (w *Worker) Connect(ctx context.Context) error {
	w.mu.Lock()
	if w.connected {
		w.mu.Unlock()
		return fmt.Errorf("already connected")
	}
	w.mu.Unlock()

	// 发送连接中状态
	w.sendStatus(event.StatusConnecting, fmt.Sprintf("正在打开串口 %s @ %d...", w.portName, w.baudRate))

	// 打开串口
	mode := &serial.Mode{
		BaudRate: w.baudRate,
	}
	port, err := serial.Open(w.portName, mode)
	if err != nil {
		w.sendStatus(event.StatusError, fmt.Sprintf("打开串口失败: %v", err))
		return fmt.Errorf("open serial port %s: %w", w.portName, err)
	}

	w.mu.Lock()
	w.port = port
	w.connected = true
	w.ctx, w.cancel = context.WithCancel(ctx)
	w.mu.Unlock()

	// 启动读取 goroutine
	w.wg.Add(1)
	go w.readLoop()

	w.sendStatus(event.StatusConnected, fmt.Sprintf("已连接 %s @ %d", w.portName, w.baudRate))
	log.Info("serial port opened", "port", w.portName, "baud", w.baudRate)

	return nil
}

// readLoop 从串口读取数据并发送到 UI。
// 当串口关闭或 context 取消时退出。
func (w *Worker) readLoop() {
	defer w.wg.Done()

	buf := make([]byte, 1024)
	for {
		// 检查 context 是否已取消
		select {
		case <-w.ctx.Done():
			return
		default:
		}

		w.mu.Lock()
		port := w.port
		w.mu.Unlock()

		if port == nil {
			break
		}

		// 设置 100ms 读取超时，避免永久阻塞
		port.SetReadTimeout(100 * time.Millisecond)

		n, err := port.Read(buf)
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
			// 超时错误是正常的，继续读取；其他错误则退出
			if !isTimeout(err) {
				log.Warn("serial read error", "err", err)
				break
			}
		}
	}

	// 更新连接状态
	w.mu.Lock()
	w.connected = false
	w.mu.Unlock()

	w.sendStatus(event.StatusDisconnected, "串口已断开")
	log.Info("serial port closed", "port", w.portName)
}

// SendInput 向串口写入数据。
// UI 层调用此方法将用户输入转发到串口设备。
func (w *Worker) SendInput(data []byte) {
	w.mu.Lock()
	port := w.port
	w.mu.Unlock()

	if port == nil {
		return
	}

	if _, err := port.Write(data); err != nil {
		log.Warn("serial write failed", "err", err)
	}
}

// Close 关闭串口并清理所有资源。
// 此方法是幂等的，可安全多次调用。
func (w *Worker) Close() {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.closed = true
	port := w.port
	w.mu.Unlock()

	// 取消 context，通知 goroutine 退出
	if w.cancel != nil {
		w.cancel()
	}

	// 关闭串口（会解除 readLoop 的阻塞）
	if port != nil {
		port.Close()
	}

	// 等待读取 goroutine 结束
	w.wg.Wait()

	log.Info("serial worker closed", "port", w.portName)
}

// IsConnected 返回当前是否已连接
func (w *Worker) IsConnected() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.connected
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

// isTimeout 判断错误是否为读取超时
func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	// go.bug.st/serial 在超时时返回特定错误
	errStr := err.Error()
	return errStr == "serial port timeout" || errStr == "timeout"
}
