package monitor

import (
	"context"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"

	"github.com/zhuyao/meatshell/internal/event"
	"github.com/zhuyao/meatshell/internal/log"
)

// LocalMonitor 使用 gopsutil 采集本机系统指标
type LocalMonitor struct {
	interval time.Duration
	uiChan   chan event.UIEvent
	tabID    string
	cancel   context.CancelFunc

	// 上次采样值（用于计算速率）
	prevNetSent   uint64
	prevNetRecv   uint64
	prevDiskRead  uint64
	prevDiskWrite uint64
	prevTime      time.Time
	prevCPUTimes  []cpu.TimesStat
}

// NewLocalMonitor 创建本机监控器，interval 为采样间隔
func NewLocalMonitor(uiChan chan event.UIEvent, tabID string, interval time.Duration) *LocalMonitor {
	if interval <= 0 {
		interval = time.Second
	}
	return &LocalMonitor{
		interval: interval,
		uiChan:   uiChan,
		tabID:    tabID,
	}
}

// Start 启动采集循环，阻塞直到 ctx 被取消或 Stop 被调用
func (m *LocalMonitor) Start(ctx context.Context) {
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
func (m *LocalMonitor) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
}

// sampleAndSend 执行一次采样并发送事件，失败时发送空数据。
// 使用非阻塞发送，避免在关闭过程中阻塞 goroutine。
func (m *LocalMonitor) sampleAndSend() {
	metrics, err := m.Sample()
	if err != nil {
		log.Warn("local monitor sample failed", "err", err)
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

// Sample 执行单次采样，返回当前系统指标
func (m *LocalMonitor) Sample() (*Metrics, error) {
	metrics := &Metrics{}

	// CPU 使用率（非阻塞，返回自上次调用以来的百分比）
	cpuPercents, err := cpu.Percent(0, false)
	if err != nil {
		return nil, err
	}
	if len(cpuPercents) > 0 {
		metrics.CPUUsage = cpuPercents[0]
	}

	// 内存
	vm, err := mem.VirtualMemory()
	if err != nil {
		return nil, err
	}
	metrics.MemTotal = vm.Total
	metrics.MemUsed = vm.Used

	// 交换空间
	sm, err := mem.SwapMemory()
	if err != nil {
		return nil, err
	}
	metrics.SwapTotal = sm.Total
	metrics.SwapUsed = sm.Used

	// 网络（聚合所有接口）
	netCounters, err := net.IOCounters(false)
	if err != nil {
		return nil, err
	}
	var curNetSent, curNetRecv uint64
	if len(netCounters) > 0 {
		curNetSent = netCounters[0].BytesSent
		curNetRecv = netCounters[0].BytesRecv
	}

	// 磁盘（汇总所有分区）
	var curDiskRead, curDiskWrite uint64
	diskCounters, err := disk.IOCounters()
	if err != nil {
		// 磁盘计数器在某些系统/权限下可能不可用，容忍错误
		log.Debug("disk IOCounters not available", "err", err)
	} else {
		for _, dc := range diskCounters {
			curDiskRead += dc.ReadBytes
			curDiskWrite += dc.WriteBytes
		}
	}

	// 计算速率
	now := time.Now()
	if !m.prevTime.IsZero() {
		elapsed := now.Sub(m.prevTime).Seconds()
		if elapsed > 0 {
			metrics.NetSent = ratePerSec(m.prevNetSent, curNetSent, elapsed)
			metrics.NetRecv = ratePerSec(m.prevNetRecv, curNetRecv, elapsed)
			metrics.DiskRead = ratePerSec(m.prevDiskRead, curDiskRead, elapsed)
			metrics.DiskWrite = ratePerSec(m.prevDiskWrite, curDiskWrite, elapsed)
		}
	}

	// 保存当前值供下次计算
	m.prevNetSent = curNetSent
	m.prevNetRecv = curNetRecv
	m.prevDiskRead = curDiskRead
	m.prevDiskWrite = curDiskWrite
	m.prevTime = now

	return metrics, nil
}

// ratePerSec 计算每秒速率，处理计数器回绕（回绕时返回 0）
func ratePerSec(prev, cur uint64, elapsed float64) uint64 {
	if cur >= prev {
		return uint64(float64(cur-prev) / elapsed)
	}
	return 0
}
