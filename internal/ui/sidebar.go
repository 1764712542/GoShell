package ui

import (
	"fmt"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/zhuyao/meatshell/internal/app"
)

// sidebarMinWidth 是侧边栏的最小宽度（像素），防止被拖拽得太窄
const sidebarMinWidth = 200

// 侧边栏颜色
var (
	colorSidebarBg     = color.RGBA{R: 0x18, G: 0x18, B: 0x25, A: 0xff}
	colorCardBg        = color.RGBA{R: 0x31, G: 0x31, B: 0x44, A: 0x60}
	colorGroupTitle    = color.RGBA{R: 0xa6, G: 0xac, B: 0xba, A: 0xff}
	colorStatusOK      = color.RGBA{R: 0xa6, G: 0xda, B: 0x95, A: 0xff}
	colorStatusError   = color.RGBA{R: 0xed, G: 0x87, B: 0x96, A: 0xff}
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
	statusDot   *canvas.Circle
}

// NewSidebar 创建侧边栏
func NewSidebar() *Sidebar {
	s := &Sidebar{
		cpuSpark:    NewSparkline(color.RGBA{R: 0xed, G: 0x87, B: 0x96, A: 0xff}, 30),
		memSpark:    NewSparkline(color.RGBA{R: 0xfa, G: 0xb3, B: 0x87, A: 0xff}, 30),
		netSpark:    NewSparkline(color.RGBA{R: 0xa6, G: 0xda, B: 0x95, A: 0xff}, 30),
		diskSpark:   NewSparkline(color.RGBA{R: 0x7b, G: 0xd3, B: 0xf5, A: 0xff}, 30),
		cpuLabel:    widget.NewLabel("CPU: --"),
		memLabel:    widget.NewLabel("MEM: --"),
		netLabel:    widget.NewLabel("NET: --"),
		diskLabel:   widget.NewLabel("DISK: --"),
		statusLabel: widget.NewLabel("就绪"),
		statusDot:   canvas.NewCircle(colorStatusError),
	}
	for _, l := range []*widget.Label{s.cpuLabel, s.memLabel, s.netLabel, s.diskLabel, s.statusLabel} {
		l.TextStyle = fyne.TextStyle{Monospace: true}
	}
	s.statusDot.Resize(fyne.NewSize(8, 8))
	s.ExtendBaseWidget(s)
	return s
}

// CreateRenderer 实现 fyne.Widget 接口
func (s *Sidebar) CreateRenderer() fyne.WidgetRenderer {
	// 分组标题
	resourcesTitle := newGroupTitle("系统资源")
	connTitle := newGroupTitle("连接状态")

	// 监控项卡片
	cpuCard := newMetricCard("CPU", s.cpuSpark, s.cpuLabel)
	memCard := newMetricCard("内存", s.memSpark, s.memLabel)
	netCard := newMetricCard("网络", s.netSpark, s.netLabel)
	diskCard := newMetricCard("磁盘", s.diskSpark, s.diskLabel)

	// 状态行（圆点 + 文字）
	statusRow := container.NewHBox(s.statusDot, s.statusLabel)

	// 整体布局：分组标题 + 卡片 + 间距
	content := container.NewVBox(
		resourcesTitle,
		cpuCard,
		memCard,
		netCard,
		diskCard,
		layout.NewSpacer(),
		connTitle,
		statusRow,
	)

	// 侧边栏背景
	bg := canvas.NewRectangle(colorSidebarBg)
	stack := container.NewStack(bg, container.NewPadded(content))

	return &sidebarRenderer{
		stack: stack,
	}
}

// sidebarRenderer 是侧边栏的自定义渲染器，
// 通过覆盖 MinSize 强制侧边栏有最小宽度，防止被拖拽得太窄。
type sidebarRenderer struct {
	stack *fyne.Container
}

func (r *sidebarRenderer) Layout(size fyne.Size) {
	r.stack.Resize(size)
}

func (r *sidebarRenderer) MinSize() fyne.Size {
	min := r.stack.MinSize()
	// 强制侧边栏最小宽度，防止 HSplit 拖拽时被压得太窄
	if min.Width < sidebarMinWidth {
		min.Width = sidebarMinWidth
	}
	return min
}

func (r *sidebarRenderer) Refresh() {
	r.stack.Refresh()
}

func (r *sidebarRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.stack}
}

func (r *sidebarRenderer) Destroy() {}

// newGroupTitle 创建分组标题
func newGroupTitle(text string) *widget.Label {
	return widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
}

// newMetricCard 创建监控项卡片（标题 + sparkline + 数值）
func newMetricCard(title string, spark *Sparkline, valueLabel *widget.Label) fyne.CanvasObject {
	// 设置 sparkline 的最小高度
	scroll := container.NewVBox(
		widget.NewLabel(title),
		spark,
		valueLabel,
	)

	// 卡片背景
	bg := canvas.NewRectangle(colorCardBg)
	return container.NewStack(bg, container.NewPadded(scroll))
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
	if connected {
		s.statusDot.FillColor = colorStatusOK
	} else {
		s.statusDot.FillColor = colorStatusError
	}
	s.statusDot.Refresh()
}

// UpdateRemoteMetrics 更新远端服务器监控指标（与本机指标叠加显示）
func (s *Sidebar) UpdateRemoteMetrics(m *app.MonitorData) {
	if m == nil {
		return
	}
	// 远端指标暂时叠加到状态行
	// 后续可扩展为独立的远端指标卡片
	s.statusLabel.SetText(fmt.Sprintf("远端 CPU: %.1f%%  MEM: %.1f%%",
		m.CPUUsage, float64(m.MemUsed)/float64(max64(m.MemTotal, 1))*100))
}

// max64 返回两个 uint64 中的较大值
func max64(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
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
