package monitor

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"
)

// PingResult 表示一次 ping 测量的结果
type PingResult struct {
	Host    string
	Latency time.Duration
	Err     error
	Time    time.Time
}

// PingMonitor 对指定主机进行周期性的 TCP 连接延迟测量。
// 不使用 ICMP（需要 root 权限），而是通过 TCP 连接握手耗时来估算网络延迟。
type PingMonitor struct {
	host    string
	port    int
	ticker  *time.Ticker
	result  chan PingResult
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	running bool
	last    *PingResult
}

// NewPingMonitor 创建一个 ping 监控器。
// host: 目标主机，port: 目标端口（通常为 SSH 端口 22）
func NewPingMonitor(host string, port int) *PingMonitor {
	return &PingMonitor{
		host: host,
		port: port,
		result: make(chan PingResult, 16),
	}
}

// Start 启动 ping 监控，按 interval 周期进行测量
func (m *PingMonitor) Start(ctx context.Context, interval time.Duration) <-chan PingResult {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return m.result
	}
	m.running = true
	m.mu.Unlock()

	m.ctx, m.cancel = context.WithCancel(ctx)
	m.ticker = time.NewTicker(interval)

	go func() {
		defer m.ticker.Stop()
		// 立即执行一次
		m.pingOnce()
		for {
			select {
			case <-m.ctx.Done():
				return
			case <-m.ticker.C:
				m.pingOnce()
			}
		}
	}()
	return m.result
}

// pingOnce 执行一次 TCP 连接延迟测量
func (m *PingMonitor) pingOnce() {
	addr := net.JoinHostPort(m.host, fmt.Sprintf("%d", m.port))
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	latency := time.Since(start)

	result := PingResult{
		Host:    m.host,
		Latency: latency,
		Err:     err,
		Time:    start,
	}

	if err == nil {
		conn.Close()
	}

	m.mu.Lock()
	m.last = &result
	m.mu.Unlock()

	select {
	case m.result <- result:
	default:
		// channel 满，丢弃旧结果
	}
}

// Stop 停止监控
func (m *PingMonitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running {
		return
	}
	m.running = false
	if m.cancel != nil {
		m.cancel()
	}
	if m.ticker != nil {
		m.ticker.Stop()
	}
}

// LastResult 返回最近一次 ping 结果
func (m *PingMonitor) LastResult() *PingResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.last
}
