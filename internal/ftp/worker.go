package ftp

import (
	"context"
	"fmt"
	"time"

	"github.com/zhuyao/meatshell/internal/event"
	"github.com/zhuyao/meatshell/internal/log"
)

// Worker 管理 FTP 连接的生命周期。
// FTP 不是终端协议，因此 Worker 仅作为连接管理器，
// 不提供终端输入/输出能力。
type Worker struct {
	host     string
	port     int
	username string
	password string
	uiChan   chan event.UIEvent
	tabID    string
	client   *Client
	ctx      context.Context
	closed   bool
}

// NewWorker 创建 FTP 连接管理器。
func NewWorker(host string, port int, username, password string, uiChan chan event.UIEvent, tabID string) *Worker {
	return &Worker{
		host:     host,
		port:     port,
		username: username,
		password: password,
		uiChan:   uiChan,
		tabID:    tabID,
	}
}

// Connect 建立 FTP 连接，并通过 uiChan 推送连接状态事件。
func (w *Worker) Connect(ctx context.Context) error {
	w.ctx = ctx
	// 检查上下文是否已取消
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("connect cancelled: %w", err)
	}

	// 发送连接中状态
	w.sendStatus(event.StatusConnecting, fmt.Sprintf("Connecting to FTP %s:%d...", w.host, w.port))

	addr := fmt.Sprintf("%s:%d", w.host, w.port)
	client, err := NewClient(addr, w.username, w.password, w.uiChan, w.tabID)
	if err != nil {
		w.sendStatus(event.StatusError, fmt.Sprintf("FTP connect failed: %v", err))
		return fmt.Errorf("connect ftp: %w", err)
	}
	w.client = client

	// 发送已连接状态
	w.sendStatus(event.StatusConnected, fmt.Sprintf("Connected to FTP %s:%d", w.host, w.port))
	return nil
}

// Client 返回 FTP 客户端用于文件操作。
func (w *Worker) Client() *Client {
	return w.client
}

// Close 关闭 FTP 连接并发送断开状态。
func (w *Worker) Close() {
	if w.closed {
		return
	}
	w.closed = true
	if w.client != nil {
		w.client.Close()
	}
	w.sendStatus(event.StatusDisconnected, "FTP disconnected")
}

// SendInput 是空操作，FTP 协议没有终端输入。
func (w *Worker) SendInput(data []byte) {
	// no-op: FTP has no terminal input
}

// Resize 是空操作，FTP 协议没有终端尺寸概念。
func (w *Worker) Resize(cols, rows int) error {
	// no-op: FTP has no terminal resize
	return nil
}


// IsConnected 返回当前是否已连接
func (w *Worker) IsConnected() bool {
	return w.client != nil && !w.closed
}

// SessionID 返回会话 ID
func (w *Worker) SessionID() string {
	return w.tabID
}

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
