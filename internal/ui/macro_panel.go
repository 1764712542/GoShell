package ui

import (
	"fmt"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// MacroRecord 表示一条已录制的宏。
//   - Name: 宏名称（用户指定或自动生成）
//   - Keys: 录制的按键序列（每个元素是一个 ANSI 转义序列或可打印字符）
//   - Timestamps: 每个按键相对于录制开始的时间偏移（用于回放时还原节奏）
type MacroRecord struct {
	Name       string
	Keys       []string
	Timestamps []time.Duration
}

// MacroPanel 是宏录制面板，提供录制/停止/播放/保存/删除等功能。
// 录制期间，所有发送到终端的按键都会被记录；回放时按原始节奏重放。
type MacroPanel struct {
	widget.BaseWidget

	mu        sync.Mutex
	records   []MacroRecord // 已保存的宏列表
	recording bool          // 是否正在录制
	current   *MacroRecord  // 当前正在录制的宏
	recordStart time.Time   // 录制开始时间

	// 回调：将按键序列发送到当前终端
	onSend func(data []byte)

	// UI 组件
	list        *widget.List
	recordBtn   *widget.Button
	stopBtn     *widget.Button
	playBtn     *widget.Button
	deleteBtn   *widget.Button
	saveBtn     *widget.Button
	nameEntry   *widget.Entry
	statusLabel *widget.Label

	// 当前选中的宏索引
	selectedIndex int

	// 通知回调（用于刷新 UI）
	onChanged func()
}

// NewMacroPanel 创建一个宏录制面板。
// onSend 回调用于在回放宏时将按键数据发送到当前终端。
func NewMacroPanel(onSend func(data []byte)) *MacroPanel {
	p := &MacroPanel{
		selectedIndex: -1,
		onSend:      onSend,
		statusLabel: widget.NewLabel("就绪"),
	}
	p.nameEntry = widget.NewEntry()
	p.nameEntry.SetPlaceHolder("宏名称")

	p.recordBtn = widget.NewButtonWithIcon("录制", theme.MediaRecordIcon(), p.StartRecording)
	p.stopBtn = widget.NewButtonWithIcon("停止", theme.MediaStopIcon(), p.StopRecording)
	p.stopBtn.Disable()
	p.playBtn = widget.NewButtonWithIcon("播放", theme.MediaPlayIcon(), p.playSelected)
	p.playBtn.Disable()
	p.deleteBtn = widget.NewButtonWithIcon("删除", theme.DeleteIcon(), p.DeleteSelected)
	p.deleteBtn.Disable()
	p.saveBtn = widget.NewButtonWithIcon("保存", theme.DocumentSaveIcon(), p.SaveCurrent)
	p.saveBtn.Disable()

	p.list = widget.NewList(
		func() int {
			p.mu.Lock()
			defer p.mu.Unlock()
			return len(p.records)
		},
		func() fyne.CanvasObject {
			return widget.NewLabel("宏名称")
		},
		func(i widget.ListItemID, obj fyne.CanvasObject) {
			p.mu.Lock()
			defer p.mu.Unlock()
			if i < 0 || i >= len(p.records) {
				return
			}
			label := obj.(*widget.Label)
			label.SetText(fmt.Sprintf("%s (%d 键)", p.records[i].Name, len(p.records[i].Keys)))
		},
	)
	p.list.OnSelected = func(id widget.ListItemID) {
		p.mu.Lock()
		p.selectedIndex = id
		p.mu.Unlock()
		p.playBtn.Enable()
		p.deleteBtn.Enable()
	}

	p.ExtendBaseWidget(p)
	return p
}

// CreateRenderer 实现 fyne.Widget 接口。
func (p *MacroPanel) CreateRenderer() fyne.WidgetRenderer {
	controls := container.NewHBox(
		p.recordBtn,
		p.stopBtn,
		p.saveBtn,
		p.nameEntry,
	)
	buttons := container.NewHBox(
		p.playBtn,
		p.deleteBtn,
	)
	content := container.NewBorder(
		container.NewVBox(controls, buttons, p.statusLabel),
		nil, nil, nil,
		p.list,
	)
	return widget.NewSimpleRenderer(content)
}

// StartRecording 开始录制宏。
// 录制期间，所有通过 RecordKey 发送的按键都会被记录。
func (p *MacroPanel) StartRecording() {
	p.mu.Lock()
	if p.recording {
		p.mu.Unlock()
		return
	}
	p.recording = true
	p.current = &MacroRecord{Name: fmt.Sprintf("macro_%s", time.Now().Format("150405"))}
	p.recordStart = time.Now()
	p.mu.Unlock()

	p.recordBtn.Disable()
	p.stopBtn.Enable()
	p.saveBtn.Disable()
	p.statusLabel.SetText("录制中...")
	if p.onChanged != nil {
		p.onChanged()
	}
}

// StopRecording 停止录制并保存当前宏。
// 如果当前宏没有按键，则丢弃。
func (p *MacroPanel) StopRecording() {
	p.mu.Lock()
	if !p.recording {
		p.mu.Unlock()
		return
	}
	p.recording = false
	current := p.current
	p.current = nil
	p.mu.Unlock()

	p.recordBtn.Enable()
	p.stopBtn.Disable()

	if current != nil && len(current.Keys) > 0 {
		p.saveBtn.Enable()
		p.statusLabel.SetText(fmt.Sprintf("已录制 %d 键，输入名称后点击保存", len(current.Keys)))
		// 临时保存到 current 以便 SaveCurrent 使用
		p.mu.Lock()
		p.current = current
		p.mu.Unlock()
	} else {
		p.statusLabel.SetText("就绪")
	}
	if p.onChanged != nil {
		p.onChanged()
	}
}

// RecordKey 记录一个按键。
// 在录制期间调用此方法会将按键添加到当前宏。
// data 为按键对应的 ANSI 序列或字符。
func (p *MacroPanel) RecordKey(data []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.recording || p.current == nil {
		return
	}
	p.current.Keys = append(p.current.Keys, string(data))
	p.current.Timestamps = append(p.current.Timestamps, time.Since(p.recordStart))
}

// IsRecording 返回是否正在录制。
func (p *MacroPanel) IsRecording() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.recording
}

// SaveCurrent 保存当前录制的宏（使用 nameEntry 中的名称）。
func (p *MacroPanel) SaveCurrent() {
	p.mu.Lock()
	current := p.current
	p.current = nil
	p.mu.Unlock()

	if current == nil || len(current.Keys) == 0 {
		return
	}
	name := p.nameEntry.Text
	if name != "" {
		current.Name = name
	}
	p.nameEntry.SetText("")

	p.mu.Lock()
	p.records = append(p.records, *current)
	p.mu.Unlock()

	p.saveBtn.Disable()
	p.statusLabel.SetText(fmt.Sprintf("已保存宏: %s", current.Name))
	p.list.Refresh()
	if p.onChanged != nil {
		p.onChanged()
	}
}

// SaveMacro 将当前录制的宏以指定名称保存。
func (p *MacroPanel) SaveMacro(name string) {
	p.mu.Lock()
	current := p.current
	p.mu.Unlock()

	if current == nil || len(current.Keys) == 0 {
		return
	}
	current.Name = name

	p.mu.Lock()
	p.records = append(p.records, *current)
	p.current = nil
	p.mu.Unlock()

	p.saveBtn.Disable()
	p.statusLabel.SetText(fmt.Sprintf("已保存宏: %s", name))
	p.list.Refresh()
	if p.onChanged != nil {
		p.onChanged()
	}
}

// playSelected 回放当前选中的宏。
func (p *MacroPanel) playSelected() {
	selected := p.selectedIndex
	if selected < 0 {
		return
	}
	p.mu.Lock()
	if selected >= len(p.records) {
		p.mu.Unlock()
		return
	}
	record := p.records[selected]
	p.mu.Unlock()

	if p.onSend == nil {
		return
	}

	p.statusLabel.SetText(fmt.Sprintf("回放中: %s", record.Name))
	go func() {
		var lastTs time.Duration
		for i, key := range record.Keys {
			if i > 0 {
				delay := record.Timestamps[i] - lastTs
				if delay > 0 && delay < 10*time.Second {
					time.Sleep(delay)
				}
			}
			lastTs = record.Timestamps[i]
			p.onSend([]byte(key))
		}
		fyne.Do(func() {
			p.statusLabel.SetText("回放完成")
		})
	}()
}

// PlayMacro 按名称查找并回放宏。
func (p *MacroPanel) PlayMacro(name string) {
	p.mu.Lock()
	var record *MacroRecord
	for i := range p.records {
		if p.records[i].Name == name {
			record = &p.records[i]
			break
		}
	}
	p.mu.Unlock()

	if record == nil || p.onSend == nil {
		return
	}

	go func() {
		var lastTs time.Duration
		for i, key := range record.Keys {
			if i > 0 {
				delay := record.Timestamps[i] - lastTs
				if delay > 0 && delay < 10*time.Second {
					time.Sleep(delay)
				}
			}
			lastTs = record.Timestamps[i]
			p.onSend([]byte(key))
		}
	}()
}

// DeleteSelected 删除当前选中的宏。
func (p *MacroPanel) DeleteSelected() {
	selected := p.selectedIndex
	if selected < 0 {
		return
	}
	p.mu.Lock()
	if selected >= len(p.records) {
		p.mu.Unlock()
		return
	}
	name := p.records[selected].Name
	p.records = append(p.records[:selected], p.records[selected+1:]...)
	p.mu.Unlock()

	p.list.UnselectAll()
	p.mu.Lock()
	p.selectedIndex = -1
	p.mu.Unlock()
	p.playBtn.Disable()
	p.deleteBtn.Disable()
	p.statusLabel.SetText(fmt.Sprintf("已删除: %s", name))
	p.list.Refresh()
	if p.onChanged != nil {
		p.onChanged()
	}
}

// DeleteMacro 按名称删除宏。
func (p *MacroPanel) DeleteMacro(name string) {
	p.mu.Lock()
	for i := range p.records {
		if p.records[i].Name == name {
			p.records = append(p.records[:i], p.records[i+1:]...)
			break
		}
	}
	p.mu.Unlock()
	p.list.Refresh()
	if p.onChanged != nil {
		p.onChanged()
	}
}

// Records 返回所有已保存宏的副本。
func (p *MacroPanel) Records() []MacroRecord {
	p.mu.Lock()
	defer p.mu.Unlock()
	result := make([]MacroRecord, len(p.records))
	copy(result, p.records)
	return result
}

// SetOnChanged 设置变更通知回调。
func (p *MacroPanel) SetOnChanged(fn func()) {
	p.onChanged = fn
}

// ShowMacroDialog 在指定窗口中显示宏面板对话框。
func ShowMacroDialog(panel *MacroPanel, win fyne.Window) {
	dialog.ShowCustom("宏录制", "关闭", panel, win)
}
