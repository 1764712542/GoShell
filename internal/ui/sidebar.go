package ui

import (
	"fmt"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/zhuyao/meatshell/internal/app"
)

// Sidebar 是系统监控侧边栏，显示本机 CPU、内存、网络和磁盘的实时指标。
type Sidebar struct {
	widget.BaseWidget
	cpuSpark    *Sparkline
	memSpark    *Sparkline
	netSpark    *Sparkline
	diskSpark   *Sparkline
	cpuLabel    *widget.Label
	memLabel    *widget.Label
	netLabel    *widget.Label
	diskLabel   *widget.Label
	statusLabel *widget.Label
}

// NewSidebar 创建侧边栏
func NewSidebar() *Sidebar {
	s := &Sidebar{
		cpuSpark:  NewSparkline(color.RGBA{R: 0xed, G: 0x87, B: 0x96, A: 0xff}, 30),
		memSpark:  NewSparkline(color.RGBA{R: 0xfa, G: 0xb3, B: 0x87, A: 0xff}, 30),
		netSpark:  NewSparkline(color.RGBA{R: 0xa6, G: 0xda, B: 0x95, A: 0xff}, 30),
		diskSpark: NewSparkline(color.RGBA{R: 0x7b, G: 0xd3, B: 0xf5, A: 0xff}, 30),
		cpuLabel:  widget.NewLabel("CPU: --"),
		memLabel:  widget.NewLabel("MEM: --"),
		netLabel:  widget.NewLabel("NET: --"),
		diskLabel: widget.NewLabel("DISK: --"),
		statusLabel: widget.NewLabel("就绪"),
	}
	for _, l := range []*widget.Label{s.cpuLabel, s.memLabel, s.netLabel, s.diskLabel, s.statusLabel} {
		l.TextStyle = fyne.TextStyle{Monospace: true}
	}
	s.ExtendBaseWidget(s)
	return s
}

// CreateRenderer 实现 fyne.Widget 接口
func (s *Sidebar) CreateRenderer() fyne.WidgetRenderer {
	content := container.NewVBox(
		widget.NewLabel("本机监控"),
		widget.NewSeparator(),
		widget.NewLabel("CPU"),
		s.cpuSpark,
		s.cpuLabel,
		widget.NewSeparator(),
		widget.NewLabel("内存"),
		s.memSpark,
		s.memLabel,
		widget.NewSeparator(),
		widget.NewLabel("网络"),
		s.netSpark,
		s.netLabel,
		widget.NewSeparator(),
		widget.NewLabel("磁盘"),
		s.diskSpark,
		s.diskLabel,
		widget.NewSeparator(),
		s.statusLabel,
	)
	return widget.NewSimpleRenderer(content)
}

// UpdateMetrics 更新监控指标
func (s *Sidebar) UpdateMetrics(m *app.MonitorData) {
	if m == nil {
		return
	}
	// CPU
	s.cpuSpark.Push(m.CPUUsage)
	s.cpuLabel.SetText(fmt.Sprintf("CPU: %.1f%%", m.CPUUsage))

	// 内存使用率
	var memPct float64
	if m.MemTotal > 0 {
		memPct = float64(m.MemUsed) / float64(m.MemTotal) * 100
	}
	s.memSpark.Push(memPct)
	s.memLabel.SetText(fmt.Sprintf("MEM: %.1f%% (%s/%s)",
		memPct, formatBytes(m.MemUsed), formatBytes(m.MemTotal)))

	// 网络（发送+接收）
	netTotal := m.NetSent + m.NetRecv
	s.netSpark.Push(float64(netTotal))
	s.netLabel.SetText(fmt.Sprintf("NET: ↓%s ↑%s",
		formatBytes(m.NetRecv), formatBytes(m.NetSent)))

	// 磁盘（读+写）
	diskTotal := m.DiskRead + m.DiskWrite
	s.diskSpark.Push(float64(diskTotal))
	s.diskLabel.SetText(fmt.Sprintf("DISK: R %s W %s",
		formatBytes(m.DiskRead), formatBytes(m.DiskWrite)))
}

// UpdateStatus 更新连接状态
func (s *Sidebar) UpdateStatus(status string, connected bool) {
	s.statusLabel.SetText(status)
}

// formatBytes 将字节数格式化为人类可读的字符串
func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	suffix := "KMGTPE"
	if exp >= len(suffix) {
		return fmt.Sprintf("%.1fPB", float64(b)/float64(div))
	}
	return fmt.Sprintf("%.1f%cB", float64(b)/float64(div), suffix[exp])
}
