package ui

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/zhuyao/meatshell/internal/sftp"
)

// SFTPView 是 SFTP 文件浏览面板，显示远程文件列表并支持导航、上传、下载。
type SFTPView struct {
	widget.BaseWidget
	browser    *sftp.Browser
	list       *widget.List
	pathEntry  *widget.Entry
	parentBtn  *widget.Button
	refreshBtn *widget.Button
	uploadBtn  *widget.Button
	mkdirBtn   *widget.Button
	win        fyne.Window

	// 当前选中的条目索引（基于 browser.Entries()，不含 ".."）
	selected int

	// 回调
	onDownload func(remotePath string)
	onUpload   func(localPath, remotePath string)
	onMkdir    func(path string)
	onRemove   func(path string)
}

// NewSFTPView 创建 SFTP 浏览面板。
// browser 为 nil 时显示占位提示，等待 SSH 连接建立后再调用 SetBrowser 设置。
func NewSFTPView(browser *sftp.Browser, win fyne.Window) *SFTPView {
	v := &SFTPView{
		browser:  browser,
		win:      win,
		selected: -1,
	}
	v.pathEntry = widget.NewEntry()
	v.pathEntry.SetPlaceHolder("当前路径")
	v.pathEntry.OnSubmitted = func(path string) {
		v.Navigate(path)
	}

	v.parentBtn = widget.NewButton("上级", func() {
		if v.browser == nil {
			return
		}
		parent, err := v.browser.Parent()
		if err != nil {
			return
		}
		v.Navigate(parent)
	})

	v.refreshBtn = widget.NewButton("刷新", func() {
		v.Refresh()
	})

	v.uploadBtn = widget.NewButton("上传", func() {
		v.showUploadDialog()
	})

	v.mkdirBtn = widget.NewButton("新建目录", func() {
		v.showMkdirDialog()
	})

	v.list = widget.NewList(
		func() int {
			if v.browser == nil {
				return 1
			}
			// +1 用于显示 ".." 返回上级
			return len(v.browser.Entries()) + 1
		},
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewIcon(theme.FolderIcon()),
				widget.NewLabel("模板条目"),
				widget.NewLabel(""),
			)
		},
		func(i widget.ListItemID, obj fyne.CanvasObject) {
			hbox := obj.(*fyne.Container)
			icon := hbox.Objects[0].(*widget.Icon)
			nameLabel := hbox.Objects[1].(*widget.Label)
			metaLabel := hbox.Objects[2].(*widget.Label)

			if v.browser == nil {
				nameLabel.SetText("（未连接）")
				metaLabel.SetText("")
				return
			}

			if i == 0 {
				// 第一行固定为 ".."
				icon.SetResource(theme.FolderIcon())
				nameLabel.SetText("..")
				metaLabel.SetText("上级目录")
				return
			}

			entries := v.browser.Entries()
			idx := i - 1
			if idx >= len(entries) {
				nameLabel.SetText("")
				metaLabel.SetText("")
				return
			}
			entry := entries[idx]
			if entry.IsDir {
				icon.SetResource(theme.FolderIcon())
				metaLabel.SetText("目录")
			} else {
				icon.SetResource(theme.FileIcon())
				metaLabel.SetText(fmt.Sprintf("%s  %s",
					formatSize(entry.Size),
					time.Unix(entry.ModTime, 0).Format("2006-01-02 15:04")))
			}
			nameLabel.SetText(entry.Name)
		},
	)

	// 双击进入目录或下载文件
	v.list.OnSelected = func(id widget.ListItemID) {
		if v.browser == nil {
			return
		}
		if id == 0 {
			// 返回上级
			parent, err := v.browser.Parent()
			if err == nil {
				v.Navigate(parent)
			}
			return
		}
		entries := v.browser.Entries()
		idx := id - 1
		if idx >= len(entries) {
			return
		}
		entry := entries[idx]
		if entry.IsDir {
			// 进入子目录
			v.Navigate(v.browser.Cwd() + "/" + entry.Name)
		}
		// 文件选中后由用户点击下载按钮
		v.selected = idx
	}

	v.ExtendBaseWidget(v)
	return v
}

// CreateRenderer 实现 fyne.Widget 接口
func (v *SFTPView) CreateRenderer() fyne.WidgetRenderer {
	// 路径栏：上级/刷新/上传/新建目录 + 路径输入
	pathBar := container.NewBorder(nil, nil,
		container.NewHBox(v.parentBtn, v.refreshBtn, v.uploadBtn, v.mkdirBtn),
		nil,
		v.pathEntry,
	)

	// 下载/删除按钮放在底部
	downloadBtn := widget.NewButton("下载选中", func() {
		v.downloadSelected()
	})
	removeBtn := widget.NewButton("删除选中", func() {
		v.removeSelected()
	})
	bottomBar := container.NewHBox(downloadBtn, removeBtn)

	content := container.NewBorder(pathBar, bottomBar, nil, nil, v.list)
	return widget.NewSimpleRenderer(content)
}

// SetBrowser 设置 SFTP 浏览器（SSH 连接成功后调用）
func (v *SFTPView) SetBrowser(browser *sftp.Browser) {
	v.browser = browser
	if browser != nil {
		v.Navigate(browser.Cwd())
	}
	v.list.Refresh()
}

// Navigate 导航到指定路径
func (v *SFTPView) Navigate(path string) {
	if v.browser == nil {
		return
	}
	if err := v.browser.List(path); err != nil {
		if v.win != nil {
			dialog.ShowError(fmt.Errorf("列出目录失败: %w", err), v.win)
		}
		return
	}
	v.pathEntry.SetText(v.browser.Cwd())
	v.selected = -1
	v.list.UnselectAll()
	v.list.Refresh()
}

// Refresh 刷新当前目录
func (v *SFTPView) Refresh() {
	if v.browser == nil {
		return
	}
	v.Navigate(v.browser.Cwd())
}

// SetOnDownload 设置下载回调
func (v *SFTPView) SetOnDownload(fn func(remotePath string)) { v.onDownload = fn }

// SetOnUpload 设置上传回调
func (v *SFTPView) SetOnUpload(fn func(localPath, remotePath string)) { v.onUpload = fn }

// SetOnMkdir 设置创建目录回调
func (v *SFTPView) SetOnMkdir(fn func(path string)) { v.onMkdir = fn }

// SetOnRemove 设置删除回调
func (v *SFTPView) SetOnRemove(fn func(path string)) { v.onRemove = fn }

// showUploadDialog 显示文件选择对话框
func (v *SFTPView) showUploadDialog() {
	if v.browser == nil || v.win == nil {
		return
	}
	dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil || reader == nil {
			return
		}
		defer reader.Close()
		localPath := reader.URI().Path()
		remotePath := v.browser.Cwd() + "/" + reader.URI().Name()
		if v.onUpload != nil {
			v.onUpload(localPath, remotePath)
		}
	}, v.win)
}

// showMkdirDialog 显示创建目录对话框
func (v *SFTPView) showMkdirDialog() {
	if v.browser == nil || v.win == nil {
		return
	}
	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder("目录名")
	dialog.ShowCustomConfirm("新建目录", "创建", "取消", nameEntry, func(confirm bool) {
		if !confirm || nameEntry.Text == "" {
			return
		}
		path := v.browser.Cwd() + "/" + nameEntry.Text
		if v.onMkdir != nil {
			v.onMkdir(path)
		}
	}, v.win)
}

// downloadSelected 下载当前选中的文件
func (v *SFTPView) downloadSelected() {
	if v.browser == nil || v.win == nil || v.selected < 0 {
		return
	}
	entries := v.browser.Entries()
	if v.selected >= len(entries) {
		return
	}
	entry := entries[v.selected]
	if entry.IsDir {
		dialog.ShowInformation("提示", "暂不支持下载目录", v.win)
		return
	}
	remotePath := v.browser.Cwd() + "/" + entry.Name
	dialog.ShowFileSave(func(writer fyne.URIWriteCloser, err error) {
		if err != nil || writer == nil {
			return
		}
		writer.Close()
		if v.onDownload != nil {
			v.onDownload(remotePath)
		}
	}, v.win)
}

// removeSelected 删除当前选中的条目
func (v *SFTPView) removeSelected() {
	if v.browser == nil || v.win == nil || v.selected < 0 {
		return
	}
	entries := v.browser.Entries()
	if v.selected >= len(entries) {
		return
	}
	entry := entries[v.selected]
	remotePath := v.browser.Cwd() + "/" + entry.Name
	dialog.ShowConfirm("确认删除", "确定要删除 "+entry.Name+" 吗？", func(ok bool) {
		if !ok {
			return
		}
		if v.onRemove != nil {
			v.onRemove(remotePath)
		}
	}, v.win)
}

// formatSize 格式化文件大小
func formatSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := int64(unit), 0
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

// 确保 SFTPView 实现了 fyne.Widget 接口（编译期检查）
var _ fyne.Widget = (*SFTPView)(nil)
