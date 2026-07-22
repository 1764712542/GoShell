package ui

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/zhuyao/meatshell/internal/sftp"
)

// localFileEntry 表示本地文件系统中的一个条目
type localFileEntry struct {
	Name    string
	IsDir   bool
	Size    int64
	ModTime time.Time
}

// SFTPView 是 SFTP 文件浏览面板，支持双窗格模式（本地+远程）。
type SFTPView struct {
	widget.BaseWidget
	browser      *sftp.Browser
	list         *widget.List
	pathEntry    *widget.Entry
	parentBtn    *widget.Button
	refreshBtn   *widget.Button
	uploadBtn    *widget.Button
	uploadDirBtn *widget.Button
	mkdirBtn     *widget.Button
	editBtn      *widget.Button
	chmodBtn     *widget.Button
	win          fyne.Window

	// 双窗格模式
	dualPaneMode bool
	localList    *widget.List
	localPath    string
	localEntries []localFileEntry

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

	v.uploadDirBtn = widget.NewButton("上传目录", func() {
		v.showUploadDirDialog()
	})

	v.mkdirBtn = widget.NewButton("新建目录", func() {
		v.showMkdirDialog()
	})

	v.editBtn = widget.NewButton("编辑", func() {
		v.editSelected()
	})

	v.chmodBtn = widget.NewButton("chmod", func() {
		v.chmodSelected()
	})

	// 初始化本地文件浏览器
	home, _ := os.UserHomeDir()
	v.localPath = home
	v.localList = v.buildLocalList()

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
	// 路径栏：上级/刷新/上传/上传目录/新建目录 + 路径输入
	pathBar := container.NewBorder(nil, nil,
		container.NewHBox(v.parentBtn, v.refreshBtn, v.uploadBtn, v.uploadDirBtn, v.mkdirBtn),
		nil,
		v.pathEntry,
	)

	// 下载/编辑/chmod/删除按钮放在底部
	downloadBtn := widget.NewButton("下载选中", func() {
		v.downloadSelected()
	})
	removeBtn := widget.NewButton("删除选中", func() {
		v.removeSelected()
	})

	// 双窗格切换按钮
	dualPaneBtn := widget.NewButton("双窗格", func() {
		v.dualPaneMode = !v.dualPaneMode
		v.Refresh()
	})
	if v.dualPaneMode {
		dualPaneBtn.Importance = widget.HighImportance
	}

	bottomBar := container.NewHBox(downloadBtn, v.editBtn, v.chmodBtn, removeBtn, dualPaneBtn)

	var remotePane fyne.CanvasObject = v.list
	if v.dualPaneMode {
		// 双窗格模式：左侧本地，右侧远程
		localLabel := widget.NewLabel("本地")
		localLabel.TextStyle = fyne.TextStyle{Bold: true}
		localPathLabel := widget.NewLabel(v.localPath)
		localPathLabel.TextStyle = fyne.TextStyle{Monospace: true}
		localPathLabel.Wrapping = fyne.TextWrapOff
		localHeader := container.NewVBox(localLabel, localPathLabel)

		remoteLabel := widget.NewLabel("远程")
		remoteLabel.TextStyle = fyne.TextStyle{Bold: true}

		localPane := container.NewBorder(localHeader, nil, nil, nil, v.localList)
		remotePane = container.NewBorder(remoteLabel, nil, nil, nil, v.list)

		split := container.NewHSplit(localPane, remotePane)
		split.SetOffset(0.5)
		content := container.NewBorder(pathBar, bottomBar, nil, nil, split)
		return widget.NewSimpleRenderer(content)
	}

	content := container.NewBorder(pathBar, bottomBar, nil, nil, remotePane)
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

// showUploadDirDialog 显示目录选择对话框，递归上传整个目录
func (v *SFTPView) showUploadDirDialog() {
	if v.browser == nil || v.win == nil {
		return
	}
	dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
		if err != nil || uri == nil {
			return
		}
		localDir := uri.Path()
		remoteDir := v.browser.Cwd() + "/" + filepath.Base(localDir)
		v.runWithProgress("上传目录", "正在上传 "+filepath.Base(localDir)+"...", func() error {
			return v.browser.UploadDir(localDir, remoteDir)
		}, true)
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

// downloadSelected 下载当前选中的文件或目录
func (v *SFTPView) downloadSelected() {
	if v.browser == nil || v.win == nil || v.selected < 0 {
		return
	}
	entries := v.browser.Entries()
	if v.selected >= len(entries) {
		return
	}
	entry := entries[v.selected]
	remotePath := v.browser.Cwd() + "/" + entry.Name

	if entry.IsDir {
		// 目录下载：选择本地目标目录后递归下载
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			localDir := filepath.Join(uri.Path(), entry.Name)
			v.runWithProgress("下载目录", "正在下载 "+entry.Name+"...", func() error {
				return v.browser.DownloadDir(remotePath, localDir)
			}, false)
		}, v.win)
		return
	}

	// 文件下载
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

// editSelected 编辑当前选中的远程文件
func (v *SFTPView) editSelected() {
	if v.browser == nil || v.win == nil || v.selected < 0 {
		return
	}
	entries := v.browser.Entries()
	if v.selected >= len(entries) {
		return
	}
	entry := entries[v.selected]
	if entry.IsDir {
		dialog.ShowInformation("提示", "无法编辑目录", v.win)
		return
	}
	remotePath := v.browser.Cwd() + "/" + entry.Name

	prog := dialog.NewProgressInfinite("编辑", "正在下载 "+entry.Name+"...", v.win)
	prog.Show()
	go func() {
		// 读取远程文件内容
		data, err := v.browser.ReadFile(remotePath)
		fyne.Do(func() { prog.Hide() })
		if err != nil {
			log.Printf("edit: read file failed: %v", err)
			fyne.Do(func() { dialog.ShowError(fmt.Errorf("读取文件失败: %w", err), v.win) })
			return
		}

		// 创建临时文件（保留扩展名以便编辑器识别）
		ext := filepath.Ext(entry.Name)
		tmpFile, err := os.CreateTemp("", "meatshell-edit-*"+ext)
		if err != nil {
			log.Printf("edit: create temp file failed: %v", err)
			return
		}
		tmpPath := tmpFile.Name()
		if _, err := tmpFile.Write(data); err != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
			log.Printf("edit: write temp file failed: %v", err)
			return
		}
		tmpFile.Close()

		// 记录初始修改时间
		info, _ := os.Stat(tmpPath)
		var lastMod time.Time
		if info != nil {
			lastMod = info.ModTime()
		}

		// 在系统默认编辑器中打开
		if err := openInEditor(tmpPath); err != nil {
			log.Printf("edit: open editor failed: %v", err)
			os.Remove(tmpPath)
			fyne.Do(func() { dialog.ShowError(fmt.Errorf("打开编辑器失败: %w", err), v.win) })
			return
		}

		// 轮询文件修改，保存后自动重新上传
		go func() {
			defer os.Remove(tmpPath)
			for i := 0; i < 3600; i++ { // 最多监视 1 小时
				time.Sleep(time.Second)
				info, err := os.Stat(tmpPath)
				if err != nil {
					return // 文件被删除，停止监视
				}
				if info.ModTime().After(lastMod) {
					lastMod = info.ModTime()
					if newData, err := os.ReadFile(tmpPath); err == nil {
						if err := v.browser.WriteFile(remotePath, newData); err != nil {
							log.Printf("edit: re-upload failed: %v", err)
						}
					}
				}
			}
		}()
	}()
}

// chmodSelected 修改当前选中条目的权限
func (v *SFTPView) chmodSelected() {
	if v.browser == nil || v.win == nil || v.selected < 0 {
		return
	}
	entries := v.browser.Entries()
	if v.selected >= len(entries) {
		return
	}
	entry := entries[v.selected]
	remotePath := v.browser.Cwd() + "/" + entry.Name

	modeEntry := widget.NewEntry()
	modeEntry.SetPlaceHolder("例如 755")
	dialog.ShowCustomConfirm("修改权限", "确定", "取消", modeEntry, func(confirm bool) {
		if !confirm || modeEntry.Text == "" {
			return
		}
		mode, err := strconv.ParseUint(modeEntry.Text, 8, 32)
		if err != nil {
			dialog.ShowError(fmt.Errorf("无效的权限值: %w", err), v.win)
			return
		}
		go func() {
			if err := v.browser.Chmod(remotePath, os.FileMode(mode)); err != nil {
				log.Printf("chmod failed: %v", err)
				fyne.Do(func() { dialog.ShowError(fmt.Errorf("修改权限失败: %w", err), v.win) })
			}
		}()
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

// runWithProgress 在显示进度条的同时执行耗时操作
func (v *SFTPView) runWithProgress(title, msg string, fn func() error, refreshAfter bool) {
	prog := dialog.NewProgressInfinite(title, msg, v.win)
	prog.Show()
	go func() {
		err := fn()
		fyne.Do(func() {
			prog.Hide()
			if err != nil {
				dialog.ShowError(fmt.Errorf("%s失败: %w", title, err), v.win)
			}
			if refreshAfter && err == nil {
				v.Refresh()
			}
		})
	}()
}

// openInEditor 使用系统默认编辑器打开文件
func openInEditor(path string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", path).Start()
	case "windows":
		return exec.Command("cmd", "/c", "start", "", path).Start()
	default:
		return exec.Command("xdg-open", path).Start()
	}
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

// buildLocalList 构建本地文件列表组件
func (v *SFTPView) buildLocalList() *widget.List {
	v.refreshLocalEntries()
	list := widget.NewList(
		func() int { return len(v.localEntries) + 1 }, // +1 for ".."
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewIcon(theme.FolderIcon()),
				widget.NewLabel("模板"),
				widget.NewLabel(""),
			)
		},
		func(i widget.ListItemID, obj fyne.CanvasObject) {
			hbox := obj.(*fyne.Container)
			icon := hbox.Objects[0].(*widget.Icon)
			nameLabel := hbox.Objects[1].(*widget.Label)
			metaLabel := hbox.Objects[2].(*widget.Label)

			if i == 0 {
				icon.SetResource(theme.FolderIcon())
				nameLabel.SetText("..")
				metaLabel.SetText("上级目录")
				return
			}
			idx := i - 1
			if idx >= len(v.localEntries) {
				return
			}
			entry := v.localEntries[idx]
			if entry.IsDir {
				icon.SetResource(theme.FolderIcon())
				metaLabel.SetText("目录")
			} else {
				icon.SetResource(theme.FileIcon())
				metaLabel.SetText(fmt.Sprintf("%s  %s",
					formatSize(entry.Size),
					entry.ModTime.Format("2006-01-02 15:04")))
			}
			nameLabel.SetText(entry.Name)
		},
	)
	list.OnSelected = func(id widget.ListItemID) {
		if id == 0 {
			parent := filepath.Dir(v.localPath)
			if parent != v.localPath {
				v.localPath = parent
				v.refreshLocalEntries()
				v.localList.Refresh()
			}
			list.UnselectAll()
			return
		}
		idx := id - 1
		if idx >= len(v.localEntries) {
			return
		}
		entry := v.localEntries[idx]
		if entry.IsDir {
			v.localPath = filepath.Join(v.localPath, entry.Name)
			v.refreshLocalEntries()
			v.localList.Refresh()
			list.UnselectAll()
			return
		}
		// 选中文件 → 触发上传
		localPath := filepath.Join(v.localPath, entry.Name)
		remotePath := ""
		if v.browser != nil {
			remotePath = v.browser.Cwd() + "/" + entry.Name
		}
		if v.onUpload != nil && remotePath != "" {
			v.onUpload(localPath, remotePath)
		}
	}
	return list
}

// refreshLocalEntries 刷新本地文件列表
func (v *SFTPView) refreshLocalEntries() {
	entries, err := os.ReadDir(v.localPath)
	if err != nil {
		v.localEntries = nil
		return
	}
	v.localEntries = make([]localFileEntry, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		v.localEntries = append(v.localEntries, localFileEntry{
			Name:    e.Name(),
			IsDir:   e.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}
	// 排序：目录优先，然后按名称
	sort.Slice(v.localEntries, func(i, j int) bool {
		if v.localEntries[i].IsDir != v.localEntries[j].IsDir {
			return v.localEntries[i].IsDir
		}
		return v.localEntries[i].Name < v.localEntries[j].Name
	})
}

// SetDualPaneMode 设置双窗格模式
func (v *SFTPView) SetDualPaneMode(enabled bool) {
	v.dualPaneMode = enabled
	v.Refresh()
}

// IsDualPaneMode 返回是否处于双窗格模式
func (v *SFTPView) IsDualPaneMode() bool {
	return v.dualPaneMode
}

// RefreshLocal 刷新本地文件列表
func (v *SFTPView) RefreshLocal() {
	v.refreshLocalEntries()
	if v.localList != nil {
		v.localList.Refresh()
	}
}
