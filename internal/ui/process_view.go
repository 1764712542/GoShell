package ui

import (
	"fmt"
	"image/color"
	"sort"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/zhuyao/meatshell/internal/app"
)

// ProcessView 是远端进程管理面板，以表格形式展示进程列表，
// 支持按 CPU 降序排序、关键字筛选、结束进程以及自动刷新。
type ProcessView struct {
	widget.BaseWidget

	entries  []app.ProcessEntry // 排序后的完整进程列表
	filtered []app.ProcessEntry // 经搜索框过滤后的列表

	search     *widget.Entry
	table      *widget.Table
	refreshBtn *widget.Button
	killBtn    *widget.Button
	summary    *widget.Label

	win       fyne.Window
	onKill    func(pid int)
	onRefresh func()

	// 当前选中行（-1 表示无选中）
	selectedRow int

	autoRefresh bool
	stopCh      chan struct{}
}

// 进程表列索引
const (
	colPID = iota
	colUser
	colName
	colCPU
	colMem
	numCols
)

// 列宽（像素）
const (
	widthPID  = 80
	widthUser = 100
	widthName = 200
	widthCPU  = 80
	widthMem  = 80
)

// autoRefreshInterval 是自动刷新的采样间隔。
const autoRefreshInterval = 5 * time.Second

// headerBg 是表头背景色。
var headerBg = color.RGBA{R: 0x31, G: 0x31, B: 0x44, A: 0xff}

// NewProcessView 创建进程管理面板。
func NewProcessView() *ProcessView {
	v := &ProcessView{
		entries:     make([]app.ProcessEntry, 0),
		filtered:    make([]app.ProcessEntry, 0),
		selectedRow: -1,
	}

	v.search = widget.NewEntry()
	v.search.SetPlaceHolder("按进程名或 PID 筛选")
	v.search.OnChanged = func(string) {
		v.applyFilter()
		v.table.Refresh()
		v.updateSummary()
	}

	v.table = widget.NewTable(
		func() (int, int) { return len(v.filtered), numCols },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.TableCellID, obj fyne.CanvasObject) {
			label := obj.(*widget.Label)
			if id.Row < 0 || id.Row >= len(v.filtered) {
				label.SetText("")
				return
			}
			e := v.filtered[id.Row]
			switch id.Col {
			case colPID:
				label.SetText(strconv.Itoa(e.PID))
			case colUser:
				label.SetText(e.User)
			case colName:
				label.SetText(e.Command)
			case colCPU:
				label.SetText(fmt.Sprintf("%.1f", e.CPU))
			case colMem:
				label.SetText(fmt.Sprintf("%.1f", e.Mem))
			default:
				label.SetText("")
			}
		},
	)
	v.table.SetColumnWidth(colPID, widthPID)
	v.table.SetColumnWidth(colUser, widthUser)
	v.table.SetColumnWidth(colName, widthName)
	v.table.SetColumnWidth(colCPU, widthCPU)
	v.table.SetColumnWidth(colMem, widthMem)

	v.table.OnSelected = func(id widget.TableCellID) {
		v.selectedRow = id.Row
	}
	v.table.OnUnselected = func(widget.TableCellID) {
		v.selectedRow = -1
	}

	v.refreshBtn = widget.NewButton("自动刷新", func() {
		v.toggleAutoRefresh()
	})

	v.killBtn = widget.NewButton("结束进程", func() {
		v.killSelected()
	})

	v.summary = widget.NewLabel("进程: 0    总CPU: 0.0%    总内存: 0.0%")

	v.ExtendBaseWidget(v)
	return v
}

// CreateRenderer 实现 fyne.Widget 接口。
func (v *ProcessView) CreateRenderer() fyne.WidgetRenderer {
	// 表头：独立的 Label 行，置于表格上方
	header := container.NewStack(
		canvas.NewRectangle(headerBg),
		container.NewPadded(container.NewHBox(
			widget.NewLabel("PID"),
			widget.NewLabel("User"),
			widget.NewLabel("Name"),
			widget.NewLabel("CPU%"),
			widget.NewLabel("Memory%"),
		)),
	)

	// 顶部工具栏：搜索框 + 自动刷新 + 结束进程
	toolbar := container.NewBorder(nil, nil,
		container.NewHBox(v.refreshBtn, v.killBtn),
		nil,
		v.search,
	)

	content := container.NewBorder(toolbar, v.summary, nil, nil,
		container.NewBorder(header, nil, nil, nil, v.table),
	)
	return widget.NewSimpleRenderer(content)
}

// UpdateProcesses 更新进程列表，按 CPU 占用降序排序后刷新表格。
func (v *ProcessView) UpdateProcesses(entries []app.ProcessEntry) {
	v.entries = make([]app.ProcessEntry, len(entries))
	copy(v.entries, entries)
	sort.SliceStable(v.entries, func(i, j int) bool {
		return v.entries[i].CPU > v.entries[j].CPU
	})
	v.applyFilter()
	v.table.Refresh()
	v.updateSummary()
}

// SetOnKill 设置结束进程的回调（由上层负责向终端发送 kill -9 <PID>）。
func (v *ProcessView) SetOnKill(fn func(pid int)) { v.onKill = fn }

// SetOnRefresh 设置自动刷新触发时调用的回调（用于请求后端重新采集进程列表）。
func (v *ProcessView) SetOnRefresh(fn func()) { v.onRefresh = fn }

// SetWindow 设置用于弹出确认对话框的父窗口。
func (v *ProcessView) SetWindow(w fyne.Window) { v.win = w }

// applyFilter 依据搜索框内容过滤进程列表。
// 搜索关键字匹配进程名（Command）、用户或 PID。
func (v *ProcessView) applyFilter() {
	q := strings.TrimSpace(strings.ToLower(v.search.Text))
	if q == "" {
		v.filtered = v.entries
		return
	}
	filtered := make([]app.ProcessEntry, 0, len(v.entries))
	for i := range v.entries {
		e := v.entries[i]
		if strings.Contains(strings.ToLower(e.Command), q) ||
			strings.Contains(strings.ToLower(e.User), q) ||
			strings.Contains(strconv.Itoa(e.PID), q) {
			filtered = append(filtered, e)
		}
	}
	v.filtered = filtered
	if v.selectedRow >= len(v.filtered) {
		v.selectedRow = -1
	}
}

// updateSummary 更新底部汇总栏：进程总数、CPU 总和、内存总和。
func (v *ProcessView) updateSummary() {
	var totalCPU, totalMem float64
	for i := range v.filtered {
		totalCPU += v.filtered[i].CPU
		totalMem += v.filtered[i].Mem
	}
	v.summary.SetText(fmt.Sprintf("进程: %d    总CPU: %.1f%%    总内存: %.1f%%",
		len(v.filtered), totalCPU, totalMem))
}

// toggleAutoRefresh 切换自动刷新状态。
func (v *ProcessView) toggleAutoRefresh() {
	if v.autoRefresh {
		if v.stopCh != nil {
			close(v.stopCh)
			v.stopCh = nil
		}
		v.autoRefresh = false
		v.refreshBtn.SetText("自动刷新")
		return
	}
	v.autoRefresh = true
	v.refreshBtn.SetText("停止刷新")
	v.stopCh = make(chan struct{})
	go v.autoRefreshLoop()
}

// autoRefreshLoop 周期性触发 onRefresh 回调，直到被停止。
func (v *ProcessView) autoRefreshLoop() {
	ticker := time.NewTicker(autoRefreshInterval)
	defer ticker.Stop()
	// 启动时立即触发一次
	if v.onRefresh != nil {
		v.onRefresh()
	}
	for {
		select {
		case <-v.stopCh:
			return
		case <-ticker.C:
			if v.onRefresh != nil {
				v.onRefresh()
			}
		}
	}
}

// killSelected 结束当前选中的进程，弹出确认对话框后调用 onKill 回调。
func (v *ProcessView) killSelected() {
	if v.selectedRow < 0 || v.selectedRow >= len(v.filtered) {
		return
	}
	pid := v.filtered[v.selectedRow].PID
	if v.win == nil {
		// 无父窗口时直接执行回调
		if v.onKill != nil {
			v.onKill(pid)
		}
		return
	}
	dialog.ShowConfirm("结束进程",
		fmt.Sprintf("确定要结束进程 PID %d 吗？(kill -9)", pid),
		func(ok bool) {
			if ok && v.onKill != nil {
				v.onKill(pid)
			}
		}, v.win)
}

// 确保 ProcessView 实现了 fyne.Widget 接口（编译期检查）
var _ fyne.Widget = (*ProcessView)(nil)
