package monitor

import "github.com/zhuyao/meatshell/internal/event"

// Metrics 表示一次采样得到的系统指标
type Metrics struct {
	CPUUsage  float64
	MemTotal  uint64
	MemUsed   uint64
	SwapTotal uint64
	SwapUsed  uint64
	NetSent   uint64 // bytes/s
	NetRecv   uint64 // bytes/s
	DiskRead  uint64 // bytes/s
	DiskWrite uint64 // bytes/s
}

// ProcessInfo 表示一个进程的信息
type ProcessInfo struct {
	PID     int
	User    string
	CPU     float64
	Mem     float64
	Command string
}

// ToMonitorData 将内部 Metrics 转换为 event.MonitorData
func (m *Metrics) ToMonitorData() *event.MonitorData {
	if m == nil {
		return nil
	}
	return &event.MonitorData{
		CPUUsage:  m.CPUUsage,
		MemTotal:  m.MemTotal,
		MemUsed:   m.MemUsed,
		SwapTotal: m.SwapTotal,
		SwapUsed:  m.SwapUsed,
		NetSent:   m.NetSent,
		NetRecv:   m.NetRecv,
		DiskRead:  m.DiskRead,
		DiskWrite: m.DiskWrite,
	}
}

// ToProcessEntry 将内部 ProcessInfo 转换为 event.ProcessEntry
func (p *ProcessInfo) ToProcessEntry() event.ProcessEntry {
	return event.ProcessEntry{
		PID:     p.PID,
		User:    p.User,
		CPU:     p.CPU,
		Mem:     p.Mem,
		Command: p.Command,
	}
}
