package monitor

import (
	"context"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/zhuyao/meatshell/internal/event"
	"github.com/zhuyao/meatshell/internal/log"
)

// cpuTimes 保存从 /proc/stat 解析出的 CPU 时间快照
type cpuTimes struct {
	total uint64
	idle  uint64
}

// RemoteMonitor 通过 SSH 采集远端系统指标
type RemoteMonitor struct {
	client   *ssh.Client
	interval time.Duration
	uiChan   chan event.UIEvent
	tabID    string
	cancel   context.CancelFunc

	// 上次采样值（用于计算速率）
	prevCPU     *cpuTimes
	prevNetSent uint64
	prevNetRecv uint64
	prevTime    time.Time
}

// NewRemoteMonitor 创建远端监控器，interval 为采样间隔
func NewRemoteMonitor(client *ssh.Client, uiChan chan event.UIEvent, tabID string, interval time.Duration) *RemoteMonitor {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	return &RemoteMonitor{
		client:   client,
		interval: interval,
		uiChan:   uiChan,
		tabID:    tabID,
	}
}

// Start 启动采集循环，阻塞直到 ctx 被取消或 Stop 被调用
func (m *RemoteMonitor) Start(ctx context.Context) {
	ctx, m.cancel = context.WithCancel(ctx)
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	// 首次采样
	m.sampleAndSend()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.sampleAndSend()
		}
	}
}

// Stop 停止采集循环
func (m *RemoteMonitor) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
}

// sampleAndSend 执行一次采样并发送事件，失败时发送空数据。
// 使用非阻塞发送，避免在关闭过程中阻塞 goroutine。
func (m *RemoteMonitor) sampleAndSend() {
	metrics, err := m.Sample()
	if err != nil {
		log.Warn("remote monitor sample failed", "err", err)
		select {
		case m.uiChan <- event.UIEvent{
			TabID:   m.tabID,
			Type:    event.EventMonitor,
			Metrics: &event.MonitorData{},
		}:
		default:
			// 通道已满或无人接收，丢弃事件
		}
		return
	}
	select {
	case m.uiChan <- event.UIEvent{
		TabID:   m.tabID,
		Type:    event.EventMonitor,
		Metrics: metrics.ToMonitorData(),
	}:
	default:
		// 通道已满或无人接收，丢弃事件
	}
}

// runCommand 在远端执行命令并返回合并输出
func (m *RemoteMonitor) runCommand(cmd string) ([]byte, error) {
	session, err := m.client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()
	return session.CombinedOutput(cmd)
}

// Sample 执行单次采样，返回远端系统指标
func (m *RemoteMonitor) Sample() (*Metrics, error) {
	metrics := &Metrics{}

	// CPU：解析 /proc/stat 的 cpu 汇总行
	curCPU, err := m.sampleCPU()
	if err != nil {
		log.Debug("remote cpu sample failed", "err", err)
	}

	// 内存和交换：解析 free -b
	if err := m.sampleMemory(metrics); err != nil {
		log.Debug("remote memory sample failed", "err", err)
	}

	// 网络：解析 /proc/net/dev
	curNetSent, curNetRecv, err := m.sampleNet()
	if err != nil {
		log.Debug("remote net sample failed", "err", err)
	}

	// 计算速率
	now := time.Now()
	if m.prevCPU != nil && curCPU != nil {
		totalDelta := curCPU.total - m.prevCPU.total
		idleDelta := curCPU.idle - m.prevCPU.idle
		if totalDelta > 0 {
			metrics.CPUUsage = (1 - float64(idleDelta)/float64(totalDelta)) * 100
		}
	}

	if !m.prevTime.IsZero() {
		elapsed := now.Sub(m.prevTime).Seconds()
		if elapsed > 0 {
			metrics.NetSent = ratePerSec(m.prevNetSent, curNetSent, elapsed)
			metrics.NetRecv = ratePerSec(m.prevNetRecv, curNetRecv, elapsed)
		}
	}

	// 保存当前值供下次计算
	m.prevCPU = curCPU
	m.prevNetSent = curNetSent
	m.prevNetRecv = curNetRecv
	m.prevTime = now

	return metrics, nil
}

// sampleCPU 解析 /proc/stat 中 cpu 汇总行，返回 total 和 idle 时间
func (m *RemoteMonitor) sampleCPU() (*cpuTimes, error) {
	out, err := m.runCommand("cat /proc/stat | grep '^cpu '")
	if err != nil {
		return nil, err
	}

	// 格式：cpu  user nice system idle iowait irq softirq steal guest guest_nice
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) < 5 {
		return nil, nil
	}
	// fields[0] = "cpu"，后续为各时间值
	var total, idle uint64
	for i := 1; i < len(fields); i++ {
		val, err := strconv.ParseUint(fields[i], 10, 64)
		if err != nil {
			return nil, err
		}
		total += val
		// idle 是第 4 个值（index 4），iowait 是第 5 个（index 5）
		if i == 4 || i == 5 {
			idle += val
		}
	}
	return &cpuTimes{total: total, idle: idle}, nil
}

// sampleMemory 解析 free -b 输出，填充内存和交换指标
func (m *RemoteMonitor) sampleMemory(metrics *Metrics) error {
	out, err := m.runCommand("free -b")
	if err != nil {
		return err
	}

	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		switch fields[0] {
		case "Mem:":
			// Mem: total used free shared buff/cache available
			metrics.MemTotal, _ = strconv.ParseUint(fields[1], 10, 64)
			metrics.MemUsed, _ = strconv.ParseUint(fields[2], 10, 64)
		case "Swap:":
			// Swap: total used free
			metrics.SwapTotal, _ = strconv.ParseUint(fields[1], 10, 64)
			metrics.SwapUsed, _ = strconv.ParseUint(fields[2], 10, 64)
		}
	}
	return nil
}

// sampleNet 解析 /proc/net/dev，汇总所有接口的字节数
func (m *RemoteMonitor) sampleNet() (sent, recv uint64, err error) {
	out, err := m.runCommand("cat /proc/net/dev")
	if err != nil {
		return 0, 0, err
	}

	lines := strings.Split(string(out), "\n")
	// 跳过前两行表头
	for i := 2; i < len(lines); i++ {
		line := lines[i]
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}

		// 接口名
		iface := strings.TrimSpace(line[:idx])
		// 跳过回环接口
		if iface == "lo" {
			continue
		}

		// 统计值
		fields := strings.Fields(line[idx+1:])
		if len(fields) < 16 {
			continue
		}
		// fields[0] = 接收字节数, fields[8] = 发送字节数
		recvBytes, _ := strconv.ParseUint(fields[0], 10, 64)
		sentBytes, _ := strconv.ParseUint(fields[8], 10, 64)
		recv += recvBytes
		sent += sentBytes
	}
	return sent, recv, nil
}
