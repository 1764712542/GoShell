package ui

import (
	"fmt"
	"image/color"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/zhuyao/meatshell/internal/config"
)

// WelcomePage 是没有打开标签页时显示的欢迎页，同时也是会话管理器。
// 参考原 Rust 项目设计：会话列表直接展示在欢迎页上，支持分组折叠与搜索过滤，
// 单击会话行即连接，单击分组标题行可折叠/展开。
type WelcomePage struct {
	widget.BaseWidget
	store *config.Store

	onConnect          func(*config.Session)
	onNewSession       func()
	onEditSession      func(*config.Session)
	onDeleteSession    func(*config.Session)
	onLocalTerm        func()
	onAbout            func()
	onDuplicateSession func(*config.Session)         // 复制会话（创建副本）
	onMoveToGroup      func(*config.Session, string) // 移动到指定分组

	sessionList      *widget.List
	searchEntry      *widget.Entry
	collapsedGroups  map[string]bool   // 分组折叠状态，key 为分组名
	filteredSessions []*config.Session // 经搜索过滤后的会话
	visibleRows      []rowMeta         // 列表可见行的元数据
}

// rowMeta 描述列表中一个可见行的元数据
type rowMeta struct {
	isHeader  bool   // 是否为分组标题行
	group     string // 所属分组名
	sessIndex int    // 会话行：在 filteredSessions 中的索引；标题行：-1
}

// NewWelcomePage 创建欢迎页
func NewWelcomePage(store *config.Store) *WelcomePage {
	w := &WelcomePage{
		store:           store,
		collapsedGroups: make(map[string]bool),
	}
	w.ExtendBaseWidget(w)
	w.searchEntry = w.buildSearchEntry()
	w.recompute()
	w.sessionList = w.buildSessionList()
	return w
}

// buildSearchEntry 构建搜索框
func (w *WelcomePage) buildSearchEntry() *widget.Entry {
	e := widget.NewEntry()
	e.SetPlaceHolder("搜索会话...")
	// 输入实时过滤名称/host/user
	e.OnChanged = func(_ string) {
		w.recompute()
		if w.sessionList != nil {
			w.sessionList.Refresh()
		}
	}
	return e
}

// recompute 根据搜索词重新计算 filteredSessions 与 visibleRows
func (w *WelcomePage) recompute() {
	var all []*config.Session
	if w.store != nil {
		all = w.store.List()
	}
	keyword := ""
	if w.searchEntry != nil {
		keyword = strings.ToLower(strings.TrimSpace(w.searchEntry.Text))
	}
	w.filteredSessions = w.filterSessions(all, keyword)
	w.visibleRows = w.buildVisibleRows(w.filteredSessions)
}

// filterSessions 按关键词过滤会话（匹配 name/host/username，不区分大小写）
func (w *WelcomePage) filterSessions(sessions []*config.Session, keyword string) []*config.Session {
	if keyword == "" {
		out := make([]*config.Session, len(sessions))
		copy(out, sessions)
		return out
	}
	out := make([]*config.Session, 0, len(sessions))
	for _, s := range sessions {
		if strings.Contains(strings.ToLower(s.Name), keyword) ||
			strings.Contains(strings.ToLower(s.Host), keyword) ||
			strings.Contains(strings.ToLower(s.Username), keyword) {
			out = append(out, s)
		}
	}
	return out
}

// buildVisibleRows 根据 filteredSessions 与折叠状态构造可见行列表。
// 行序：分组标题行 + 该分组下（未折叠时）的会话行。
func (w *WelcomePage) buildVisibleRows(sessions []*config.Session) []rowMeta {
	groups := w.orderedGroups(sessions)

	rows := make([]rowMeta, 0, len(sessions)+len(groups))
	for _, g := range groups {
		// 收集属于该分组的会话索引（sessions 已按 group/name 排序）
		var idxs []int
		for i, s := range sessions {
			if s.Group == g {
				idxs = append(idxs, i)
			}
		}
		if len(idxs) == 0 {
			continue
		}
		// 标题行
		rows = append(rows, rowMeta{isHeader: true, group: g, sessIndex: -1})
		if w.collapsedGroups[g] {
			continue
		}
		for _, i := range idxs {
			rows = append(rows, rowMeta{isHeader: false, group: g, sessIndex: i})
		}
	}
	return rows
}

// orderedGroups 返回 sessions 中出现的分组，按字母序排列，未分组（""）放最后
func (w *WelcomePage) orderedGroups(sessions []*config.Session) []string {
	groupSet := make(map[string]bool)
	for _, s := range sessions {
		groupSet[s.Group] = true
	}
	groups := make([]string, 0, len(groupSet))
	for g := range groupSet {
		if g != "" {
			groups = append(groups, g)
		}
	}
	sort.Strings(groups)
	if groupSet[""] {
		groups = append(groups, "")
	}
	return groups
}

// resolveRow 将可见索引映射到行类型（isHeader, group, sessIndex）
func (w *WelcomePage) resolveRow(visibleIndex int) (isHeader bool, group string, sessIndex int) {
	if visibleIndex < 0 || visibleIndex >= len(w.visibleRows) {
		return false, "", -1
	}
	r := w.visibleRows[visibleIndex]
	return r.isHeader, r.group, r.sessIndex
}

// toggleGroup 切换分组的折叠状态并刷新列表
func (w *WelcomePage) toggleGroup(group string) {
	if w.collapsedGroups[group] {
		delete(w.collapsedGroups, group)
	} else {
		w.collapsedGroups[group] = true
	}
	w.visibleRows = w.buildVisibleRows(w.filteredSessions)
	if w.sessionList != nil {
		w.sessionList.Refresh()
	}
}

// groupDisplay 返回分组的显示名称（空分组显示为“未分组”）
func groupDisplay(g string) string {
	if g == "" {
		return "未分组"
	}
	return g
}

// sessionIconText 返回会话类型对应的图标字符
func sessionIconText(t config.SessionType) string {
	switch t {
	case config.SessionSSH:
		return "▶"
	case config.SessionSerial:
		return "◇"
	case config.SessionTelnet:
		return "◈"
	case config.SessionLocal:
		return "■"
	case config.SessionRLogin:
		return "◆"
	case config.SessionFTP:
		return "▦"
	case config.SessionMosh:
		return "◊"
	default:
		return "▣"
	}
}

// buildSessionList 构建会话列表（含分组标题行与会话行）
// 由于 widget.List 不支持分组头部，采用标题行 + 会话行混排方案：
// 每行模板包含标题行与会话行两层（Stack 叠放），按行类型切换可见性。
func (w *WelcomePage) buildSessionList() *widget.List {
	list := widget.NewList(
		func() int { return len(w.visibleRows) },
		// 模板
		func() fyne.CanvasObject {
			// ---- 标题行：▶/▼ + 分组名 + 会话数 ----
			arrow := canvas.NewText("▼", color.RGBA{R: 0x89, G: 0xb4, B: 0xfa, A: 0xff})
			arrow.TextSize = 14
			arrow.TextStyle = fyne.TextStyle{Bold: true}
			groupLabel := widget.NewLabel("分组名")
			groupLabel.TextStyle = fyne.TextStyle{Bold: true}
			countLabel := widget.NewLabel("(0)")
			countLabel.TextStyle = fyne.TextStyle{Bold: true}
			headerRow := container.NewHBox(arrow, groupLabel, countLabel)

			// ---- 会话行：[图标] [名称] [user@host:port] ←留白→ [编辑][删除][...] ----
			icon := canvas.NewText("▶", color.RGBA{R: 0x89, G: 0xb4, B: 0xfa, A: 0xff})
			icon.TextSize = 16
			nameLabel := widget.NewLabel("会话名称")
			nameLabel.TextStyle = fyne.TextStyle{Bold: true}
			hostLabel := widget.NewLabel("user@host:port")
			hostLabel.TextStyle = fyne.TextStyle{Monospace: true}
			editBtn := widget.NewButton("编辑", nil)
			delBtn := widget.NewButton("删除", nil)
			delBtn.Importance = widget.DangerImportance
			// “...” 按钮弹出右键菜单（Fyne List 不直接支持右键，改用按钮触发）
			moreBtn := widget.NewButtonWithIcon("", theme.MoreHorizontalIcon(), nil)
			sessionRow := container.NewHBox(icon, nameLabel, hostLabel, layout.NewSpacer(), editBtn, delBtn, moreBtn)

			// Stack 叠放：更新时按行类型隐藏另一层
			return container.NewStack(headerRow, sessionRow)
		},
		// 更新
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			isHeader, group, sessIndex := w.resolveRow(id)
			stack := obj.(*fyne.Container)
			headerRow := stack.Objects[0].(*fyne.Container)
			sessionRow := stack.Objects[1].(*fyne.Container)

			if isHeader {
				sessionRow.Hide()
				headerRow.Show()
				arrow := headerRow.Objects[0].(*canvas.Text)
				groupLabel := headerRow.Objects[1].(*widget.Label)
				countLabel := headerRow.Objects[2].(*widget.Label)

				if w.collapsedGroups[group] {
					arrow.Text = "▶"
				} else {
					arrow.Text = "▼"
				}
				groupLabel.SetText(groupDisplay(group))
				count := 0
				for _, s := range w.filteredSessions {
					if s.Group == group {
						count++
					}
				}
				countLabel.SetText(fmt.Sprintf("(%d)", count))
				return
			}

			// 会话行
			headerRow.Hide()
			sessionRow.Show()
			if sessIndex < 0 || sessIndex >= len(w.filteredSessions) {
				return
			}
			sess := w.filteredSessions[sessIndex]

			icon := sessionRow.Objects[0].(*canvas.Text)
			nameLabel := sessionRow.Objects[1].(*widget.Label)
			hostLabel := sessionRow.Objects[2].(*widget.Label)
			// Objects[3] 是 layout.Spacer()
			editBtn := sessionRow.Objects[4].(*widget.Button)
			delBtn := sessionRow.Objects[5].(*widget.Button)
			moreBtn := sessionRow.Objects[6].(*widget.Button)

			// 图标
			icon.Text = sessionIconText(sess.Type)

			// 名称
			nameLabel.SetText(sess.Name)

			// host 信息
			if sess.Type == config.SessionLocal {
				if sess.Shell != "" {
					hostLabel.SetText(sess.Shell)
				} else {
					hostLabel.SetText("系统默认 Shell")
				}
			} else {
				parts := ""
				if sess.Username != "" {
					parts = sess.Username + "@"
				}
				parts += sess.Host
				if sess.Port > 0 {
					parts += fmt.Sprintf(":%d", sess.Port)
				}
				hostLabel.SetText(parts)
			}

			s := sess
			editBtn.OnTapped = func() {
				if w.onEditSession != nil {
					w.onEditSession(s)
				}
			}
			delBtn.OnTapped = func() {
				if w.onDeleteSession != nil {
					w.onDeleteSession(s)
				}
			}
			moreBtn.OnTapped = func() {
				w.showSessionMenu(s, moreBtn)
			}
		},
	)

	// 单击行：标题行折叠/展开，会话行连接
	list.OnSelected = func(id widget.ListItemID) {
		list.UnselectAll()
		isHeader, group, sessIndex := w.resolveRow(id)
		if isHeader {
			w.toggleGroup(group)
			return
		}
		if sessIndex < 0 || sessIndex >= len(w.filteredSessions) {
			return
		}
		if w.onConnect != nil {
			w.onConnect(w.filteredSessions[sessIndex])
		}
	}

	return list
}

// showSessionMenu 弹出会话操作菜单（连接/编辑/复制/移动到分组/删除）
func (w *WelcomePage) showSessionMenu(sess *config.Session, anchor fyne.CanvasObject) {
	c := fyne.CurrentApp().Driver().CanvasForObject(anchor)
	if c == nil {
		return
	}

	items := []*fyne.MenuItem{
		fyne.NewMenuItem("连接", func() {
			if w.onConnect != nil {
				w.onConnect(sess)
			}
		}),
		fyne.NewMenuItem("编辑", func() {
			if w.onEditSession != nil {
				w.onEditSession(sess)
			}
		}),
		fyne.NewMenuItem("复制", func() {
			if w.onDuplicateSession != nil {
				w.onDuplicateSession(sess)
			}
		}),
	}

	// 移动到分组子菜单：列出已有分组 + “新建分组...”
	moveItems := make([]*fyne.MenuItem, 0, 8)
	for _, g := range w.allGroupNames() {
		g := g // capture
		label := groupDisplay(g)
		if sess.Group == g {
			label = "✓ " + label
		}
		moveItems = append(moveItems, fyne.NewMenuItem(label, func() {
			if w.onMoveToGroup != nil {
				w.onMoveToGroup(sess, g)
			}
		}))
	}
	moveItems = append(moveItems, fyne.NewMenuItemSeparator())
	moveItems = append(moveItems, fyne.NewMenuItem("新建分组...", func() {
		w.promptNewGroup(sess)
	}))
	moveMenu := fyne.NewMenu("移动到分组", moveItems...)
	items = append(items, &fyne.MenuItem{Label: "移动到分组", ChildMenu: moveMenu})

	items = append(items, fyne.NewMenuItem("删除", func() {
		if w.onDeleteSession != nil {
			w.onDeleteSession(sess)
		}
	}))

	menu := fyne.NewMenu("会话操作", items...)
	widget.ShowPopUpMenuAtRelativePosition(menu, c, fyne.NewPos(0, anchor.Size().Height), anchor)
}

// allGroupNames 返回所有已有分组（按字母序，不含未分组）
func (w *WelcomePage) allGroupNames() []string {
	groupSet := make(map[string]bool)
	if w.store != nil {
		for _, s := range w.store.List() {
			if s.Group != "" {
				groupSet[s.Group] = true
			}
		}
	}
	groups := make([]string, 0, len(groupSet))
	for g := range groupSet {
		groups = append(groups, g)
	}
	sort.Strings(groups)
	return groups
}

// promptNewGroup 弹出输入对话框获取新分组名，并触发移动
func (w *WelcomePage) promptNewGroup(sess *config.Session) {
	win := windowForObject(w)
	if win == nil {
		return
	}
	dialog.ShowEntryDialog("新建分组", "请输入分组名称：", func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if w.onMoveToGroup != nil {
			w.onMoveToGroup(sess, name)
		}
	}, win)
}

// windowForObject 从 widget 查找其所在的 fyne.Window
func windowForObject(obj fyne.CanvasObject) fyne.Window {
	c := fyne.CurrentApp().Driver().CanvasForObject(obj)
	if c == nil {
		return nil
	}
	for _, win := range fyne.CurrentApp().Driver().AllWindows() {
		if win.Canvas() == c {
			return win
		}
	}
	return nil
}

// RefreshSessions 刷新会话列表（重新计算过滤结果）
func (w *WelcomePage) RefreshSessions() {
	w.recompute()
	if w.sessionList != nil {
		w.sessionList.Refresh()
	}
}

// CreateRenderer 实现 fyne.Widget 接口
func (w *WelcomePage) CreateRenderer() fyne.WidgetRenderer {
	// 标题
	title := canvas.NewText("GoShell", color.RGBA{R: 0x89, G: 0xb4, B: 0xfa, A: 0xff})
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.TextSize = 28
	title.Alignment = fyne.TextAlignCenter

	subtitle := canvas.NewText("轻量级 SSH / 串口 / Telnet 终端客户端", color.RGBA{R: 0xa6, G: 0xac, B: 0xba, A: 0xff})
	subtitle.TextSize = 14
	subtitle.Alignment = fyne.TextAlignCenter

	// 工具栏
	newBtn := widget.NewButtonWithIcon("新建会话", theme.ContentAddIcon(), func() {
		if w.onNewSession != nil {
			w.onNewSession()
		}
	})
	newBtn.Importance = widget.HighImportance

	localBtn := widget.NewButtonWithIcon("本地终端", theme.ComputerIcon(), func() {
		if w.onLocalTerm != nil {
			w.onLocalTerm()
		}
	})

	aboutBtn := widget.NewButtonWithIcon("关于", theme.InfoIcon(), func() {
		if w.onAbout != nil {
			w.onAbout()
		}
	})

	toolbar := container.NewHBox(newBtn, localBtn, aboutBtn)

	// 搜索框（位于工具栏下方）
	searchBox := container.NewPadded(w.searchEntry)

	// 列表区域
	hasSessions := w.store != nil && len(w.store.List()) > 0
	var listArea fyne.CanvasObject
	if hasSessions {
		listArea = w.sessionList
	} else {
		emptyHint := widget.NewLabel("暂无会话，点击「新建会话」添加第一台服务器")
		emptyHint.Alignment = fyne.TextAlignCenter
		listArea = container.NewCenter(emptyHint)
	}

	// 整体布局：顶部标题+工具栏+搜索框，下方列表
	content := container.NewBorder(
		container.NewVBox(
			container.NewPadded(container.NewCenter(container.NewVBox(title, subtitle))),
			container.NewPadded(toolbar),
			searchBox,
			widget.NewSeparator(),
		),
		nil, nil, nil,
		listArea,
	)

	return widget.NewSimpleRenderer(content)
}

// SetOnConnect 设置连接会话回调（点击会话行触发）
func (w *WelcomePage) SetOnConnect(fn func(*config.Session)) { w.onConnect = fn }

// SetOnNewSession 设置新建会话回调
func (w *WelcomePage) SetOnNewSession(fn func()) { w.onNewSession = fn }

// SetOnEditSession 设置编辑会话回调
func (w *WelcomePage) SetOnEditSession(fn func(*config.Session)) { w.onEditSession = fn }

// SetOnDeleteSession 设置删除会话回调
func (w *WelcomePage) SetOnDeleteSession(fn func(*config.Session)) { w.onDeleteSession = fn }

// SetOnLocalTerm 设置本地终端回调
func (w *WelcomePage) SetOnLocalTerm(fn func()) { w.onLocalTerm = fn }

// SetOnAbout 设置关于回调
func (w *WelcomePage) SetOnAbout(fn func()) { w.onAbout = fn }

// SetOnDuplicateSession 设置复制会话回调（创建副本，名称后加 "_copy"）
func (w *WelcomePage) SetOnDuplicateSession(fn func(*config.Session)) { w.onDuplicateSession = fn }

// SetOnMoveToGroup 设置移动会话到分组回调
func (w *WelcomePage) SetOnMoveToGroup(fn func(*config.Session, string)) { w.onMoveToGroup = fn }

// ShowDeleteConfirm 显示删除确认对话框
func ShowDeleteConfirm(sess *config.Session, store *config.Store, win fyne.Window, onDeleted func()) {
	dialog.ShowConfirm("确认删除", "确定要删除会话「"+sess.Name+"」吗？", func(ok bool) {
		if !ok {
			return
		}
		if err := store.Delete(sess.ID); err != nil {
			dialog.ShowError(err, win)
			return
		}
		if onDeleted != nil {
			onDeleted()
		}
	}, win)
}

var _ fyne.Widget = (*WelcomePage)(nil)
