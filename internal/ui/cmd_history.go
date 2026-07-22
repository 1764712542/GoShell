package ui

import (
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// CmdHistoryEntry represents a single executed command in the history.
type CmdHistoryEntry struct {
	Command   string
	Timestamp time.Time
	TabID     string
	TabTitle  string
}

// CmdHistoryPanel displays a searchable list of executed commands.
// Commands can be re-run by clicking, or copied/deleted via a per-row menu.
type CmdHistoryPanel struct {
	widget.BaseWidget
	history     []CmdHistoryEntry
	filtered    []int // indices into history matching the current search
	list        *widget.List
	searchEntry *widget.Entry
	onSelect    func(cmd string)
	maxSize     int
	searchKW    string
}

// NewCmdHistoryPanel creates a new command history panel with a default
// maximum size of 500 entries.
func NewCmdHistoryPanel() *CmdHistoryPanel {
	p := &CmdHistoryPanel{
		history:  make([]CmdHistoryEntry, 0),
		filtered: make([]int, 0),
		maxSize:  500,
	}

	p.searchEntry = widget.NewEntry()
	p.searchEntry.SetPlaceHolder("搜索命令...")
	p.searchEntry.OnChanged = func(s string) {
		p.Search(s)
	}

	p.list = widget.NewList(
		func() int { return len(p.filtered) },
		func() fyne.CanvasObject {
			tsLabel := widget.NewLabel("[00:00:00]")
			tsLabel.TextStyle = fyne.TextStyle{Monospace: true}

			cmdLabel := widget.NewLabel("command")
			cmdLabel.Truncation = fyne.TextTruncateEllipsis

			tabLabel := widget.NewLabel("tab")
			tabLabel.TextStyle = fyne.TextStyle{Italic: true}

			infoBox := container.NewVBox(cmdLabel, tabLabel)
			moreBtn := widget.NewButton("⋯", nil)

			return container.NewHBox(tsLabel, infoBox, layout.NewSpacer(), moreBtn)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id < 0 || id >= len(p.filtered) {
				return
			}
			entry := p.history[p.filtered[id]]

			hbox := obj.(*fyne.Container)
			tsLabel := hbox.Objects[0].(*widget.Label)
			infoBox := hbox.Objects[1].(*fyne.Container)
			cmdLabel := infoBox.Objects[0].(*widget.Label)
			tabLabel := infoBox.Objects[1].(*widget.Label)
			moreBtn := hbox.Objects[3].(*widget.Button)

			tsLabel.SetText("[" + entry.Timestamp.Format("15:04:05") + "]")
			cmdLabel.SetText(entry.Command)
			tabLabel.SetText(entry.TabTitle)

			moreBtn.OnTapped = func() {
				p.showEntryMenu(p.filtered[id], moreBtn)
			}
		},
	)

	// Clicking a row triggers the re-run callback.
	p.list.OnSelected = func(id widget.ListItemID) {
		if id < 0 || id >= len(p.filtered) {
			return
		}
		entry := p.history[p.filtered[id]]
		p.list.UnselectAll()
		if p.onSelect != nil {
			p.onSelect(entry.Command)
		}
	}

	p.ExtendBaseWidget(p)
	return p
}

// CreateRenderer implements fyne.Widget.
func (p *CmdHistoryPanel) CreateRenderer() fyne.WidgetRenderer {
	clearBtn := widget.NewButtonWithIcon("清空", theme.ContentClearIcon(), func() {
		p.Clear()
	})
	exportBtn := widget.NewButtonWithIcon("导出", theme.DocumentSaveIcon(), func() {
		p.showExportDialog()
	})

	toolbar := container.NewBorder(nil, nil, nil,
		container.NewHBox(clearBtn, exportBtn),
		p.searchEntry,
	)

	content := container.NewBorder(toolbar, nil, nil, nil, p.list)
	return widget.NewSimpleRenderer(content)
}

// AddCommand adds a command to the history. Consecutive duplicates of the
// same command are ignored. When the history reaches maxSize, the oldest
// entry is discarded.
func (p *CmdHistoryPanel) AddCommand(cmd string, tabID string, tabTitle string) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return
	}
	// Deduplicate consecutive duplicates.
	if len(p.history) > 0 && p.history[len(p.history)-1].Command == cmd {
		return
	}

	p.history = append(p.history, CmdHistoryEntry{
		Command:   cmd,
		Timestamp: time.Now(),
		TabID:     tabID,
		TabTitle:  tabTitle,
	})

	// Enforce maxSize.
	if len(p.history) > p.maxSize {
		p.history = p.history[len(p.history)-p.maxSize:]
	}

	p.rebuildFilter()
	p.list.Refresh()
}

// Clear removes all history entries.
func (p *CmdHistoryPanel) Clear() {
	p.history = p.history[:0]
	p.filtered = p.filtered[:0]
	p.list.Refresh()
}

// SetOnSelect sets the callback invoked when the user clicks a command to
// re-run it.
func (p *CmdHistoryPanel) SetOnSelect(fn func(cmd string)) {
	p.onSelect = fn
}

// Export returns the history as plain text, one command per line.
func (p *CmdHistoryPanel) Export() string {
	if len(p.history) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, e := range p.history {
		sb.WriteString(e.Command)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// Search filters the displayed history by command text (case-insensitive
// substring match). An empty keyword shows all entries.
func (p *CmdHistoryPanel) Search(keyword string) {
	p.searchKW = keyword
	p.rebuildFilter()
	p.list.Refresh()
}

// rebuildFilter recomputes the filtered index slice based on searchKW.
func (p *CmdHistoryPanel) rebuildFilter() {
	p.filtered = p.filtered[:0]
	kw := strings.ToLower(p.searchKW)
	for i, e := range p.history {
		if kw == "" || strings.Contains(strings.ToLower(e.Command), kw) {
			p.filtered = append(p.filtered, i)
		}
	}
}

// showEntryMenu shows a popup menu with Copy, Re-run, and Delete options
// for the entry at the given history index.
func (p *CmdHistoryPanel) showEntryMenu(histIdx int, btn *widget.Button) {
	if histIdx < 0 || histIdx >= len(p.history) {
		return
	}
	entry := p.history[histIdx]
	menu := fyne.NewMenu("",
		fyne.NewMenuItem("复制", func() {
			p.copyToClipboard(entry.Command)
		}),
		fyne.NewMenuItem("重新运行", func() {
			if p.onSelect != nil {
				p.onSelect(entry.Command)
			}
		}),
		fyne.NewMenuItem("删除", func() {
			p.deleteEntry(histIdx)
		}),
	)
	c := fyne.CurrentApp().Driver().CanvasForObject(btn)
	if c != nil {
		widget.ShowPopUpMenuAtRelativePosition(menu, c,
			fyne.NewPos(0, btn.Size().Height), btn)
	}
}

// deleteEntry removes the entry at the given history index and refreshes
// the filter and list.
func (p *CmdHistoryPanel) deleteEntry(histIdx int) {
	if histIdx < 0 || histIdx >= len(p.history) {
		return
	}
	p.history = append(p.history[:histIdx], p.history[histIdx+1:]...)
	p.rebuildFilter()
	p.list.Refresh()
}

// copyToClipboard copies the given text to the system clipboard.
func (p *CmdHistoryPanel) copyToClipboard(text string) {
	wins := fyne.CurrentApp().Driver().AllWindows()
	if len(wins) > 0 {
		wins[0].Clipboard().SetContent(text)
	}
}

// showExportDialog displays the exported history in a dialog.
func (p *CmdHistoryPanel) showExportDialog() {
	text := p.Export()
	if text == "" {
		return
	}
	entry := widget.NewMultiLineEntry()
	entry.SetText(text)
	entry.SetMinRowsVisible(10)

	wins := fyne.CurrentApp().Driver().AllWindows()
	if len(wins) == 0 {
		return
	}
	dialog.ShowCustom("导出历史", "关闭", entry, wins[0])
}

// Ensure CmdHistoryPanel implements fyne.Widget.
var _ fyne.Widget = (*CmdHistoryPanel)(nil)
