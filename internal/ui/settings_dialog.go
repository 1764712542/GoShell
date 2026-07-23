package ui

import (
	"fmt"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/zhuyao/meatshell/internal/config"
)

// SettingsDialog 是全局设置对话框
type SettingsDialog struct {
	win   fyne.Window
	pref  fyne.Preferences
	prefs *config.PreferencesManager
	store *config.Store

	// 通用标签页控件
	generalFontSize   *widget.Slider
	generalCols       *widget.Entry
	generalRows       *widget.Entry
	generalScheme     *widget.Select
	generalScrollback *widget.Entry

	// SSH 标签页控件
	sshKeepalive *widget.Slider
	sshMaxFails  *widget.Slider
	sshPort      *widget.Entry
	sshPolicy    *widget.Select

	// 代理标签页控件
	proxySocks5 *widget.Entry
	proxyHTTP   *widget.Entry
	proxyBypass *widget.Entry

	// 安全标签页控件
	securityMasterEnabled *widget.Check
	securityAutoLock      *widget.Slider
}

// Preference keys 常量
const (
	prefFontSize          = "fontSize"
	prefTerminalCols      = "terminalCols"
	prefTerminalRows      = "terminalRows"
	prefColorScheme       = "colorScheme"
	prefScrollbackSize    = "scrollbackSize"
	prefKeepaliveInterval = "keepaliveInterval"
	prefMaxKeepaliveFails = "maxKeepaliveFailures"
	prefDefaultSSHPort    = "defaultSSHPort"
	prefKnownHostsPolicy  = "knownHostsPolicy"
	prefGlobalSocks5Proxy = "globalSocks5Proxy"
	prefGlobalHTTPProxy   = "globalHTTPProxy"
	prefProxyBypassList   = "proxyBypassList"
	prefMasterPasswordOn  = "masterPasswordEnabled"
	prefAutoLockTimeout   = "autoLockTimeout"
)

// Default values
const (
	defaultFontSize          = 14.0
	defaultTerminalCols      = 80
	defaultTerminalRows      = 24
	defaultScrollbackSize    = 10000
	defaultKeepaliveInterval = 30
	defaultMaxKeepaliveFails = 3
	defaultSSHPort           = 22
	defaultAutoLockTimeout   = 5
)

// knownHostsPolicies 列出已知主机策略选项
var knownHostsPolicies = []string{"auto-accept", "strict", "ask"}

// shortcutActionOrder 定义快捷键在设置界面的展示顺序
var shortcutActionOrder = []string{
	"newTab", "closeTab", "nextTab", "prevTab",
	"copy", "paste", "search", "settings",
	"toggleSFTP", "fontSizeUp", "fontSizeDown", "fontSizeReset",
}

// shortcutActionLabels 将快捷键动作映射为可读标签
var shortcutActionLabels = map[string]string{
	"newTab":        "新建标签页",
	"closeTab":      "关闭标签页",
	"nextTab":       "下一个标签页",
	"prevTab":       "上一个标签页",
	"copy":          "复制",
	"paste":         "粘贴",
	"search":        "搜索",
	"settings":      "设置",
	"toggleSFTP":    "切换 SFTP",
	"fontSizeUp":    "放大字号",
	"fontSizeDown":  "缩小字号",
	"fontSizeReset": "重置字号",
}

// ShowSettingsDialog 显示全局设置对话框
func ShowSettingsDialog(win fyne.Window, store *config.Store, prefs *config.PreferencesManager) {
	d := &SettingsDialog{
		win:   win,
		pref:  fyne.CurrentApp().Preferences(),
		prefs: prefs,
		store: store,
	}

	tabs := container.NewAppTabs(
		container.NewTabItem("通用", d.buildGeneralTab()),
		container.NewTabItem("SSH", d.buildSSHTab()),
		container.NewTabItem("代理", d.buildProxyTab()),
		container.NewTabItem("快捷键", d.buildShortcutsTab()),
		container.NewTabItem("安全", d.buildSecurityTab()),
	)
	tabs.SetTabLocation(container.TabLocationTop)

	dialog.ShowCustomConfirm("设置", "确定", "取消", tabs, func(confirmed bool) {
		if confirmed {
			d.save()
		}
	}, win)
}

// buildGeneralTab 构建通用设置标签页
func (d *SettingsDialog) buildGeneralTab() fyne.CanvasObject {
	cur := d.prefs.Get()
	fontSize := float64(cur.Font.Size)
	if fontSize <= 0 {
		fontSize = defaultFontSize
	}
	d.generalFontSize = widget.NewSlider(10, 24)
	d.generalFontSize.Value = fontSize
	fontSizeLabel := widget.NewLabel(fmt.Sprintf("%.0f", fontSize))
	d.generalFontSize.OnChanged = func(v float64) {
		fontSizeLabel.SetText(fmt.Sprintf("%.0f", v))
	}

	d.generalCols = widget.NewEntry()
	d.generalCols.SetText(strconv.Itoa(d.pref.IntWithFallback(prefTerminalCols, defaultTerminalCols)))
	d.generalRows = widget.NewEntry()
	d.generalRows.SetText(strconv.Itoa(d.pref.IntWithFallback(prefTerminalRows, defaultTerminalRows)))

	schemes := GetColorSchemes()
	schemeNames := make([]string, len(schemes))
	for i, s := range schemes {
		schemeNames[i] = s.Name
	}
	currentScheme := cur.Theme.Name
	if currentScheme == "" {
		if def := DefaultColorScheme(); def != nil {
			currentScheme = def.Name
		}
	}
	d.generalScheme = widget.NewSelect(schemeNames, nil)
	d.generalScheme.SetSelected(currentScheme)

	scrollback := cur.Terminal.ScrollbackLines
	if scrollback <= 0 {
		scrollback = defaultScrollbackSize
	}
	d.generalScrollback = widget.NewEntry()
	d.generalScrollback.SetText(strconv.Itoa(scrollback))

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "默认字体大小", Widget: container.NewHBox(d.generalFontSize, fontSizeLabel)},
			{Text: "终端列数", Widget: d.generalCols},
			{Text: "终端行数", Widget: d.generalRows},
			{Text: "默认配色方案", Widget: d.generalScheme},
			{Text: "回滚缓冲区大小", Widget: d.generalScrollback},
		},
	}

	return container.NewVScroll(form)
}

// buildSSHTab 构建 SSH 设置标签页
func (d *SettingsDialog) buildSSHTab() fyne.CanvasObject {
	keepalive := d.pref.IntWithFallback(prefKeepaliveInterval, defaultKeepaliveInterval)
	d.sshKeepalive = widget.NewSlider(10, 120)
	d.sshKeepalive.Value = float64(keepalive)
	keepaliveLabel := widget.NewLabel(fmt.Sprintf("%ds", keepalive))
	d.sshKeepalive.OnChanged = func(v float64) {
		keepaliveLabel.SetText(fmt.Sprintf("%ds", int(v)))
	}

	maxFails := d.pref.IntWithFallback(prefMaxKeepaliveFails, defaultMaxKeepaliveFails)
	d.sshMaxFails = widget.NewSlider(1, 10)
	d.sshMaxFails.Value = float64(maxFails)
	maxFailsLabel := widget.NewLabel(fmt.Sprintf("%d", maxFails))
	d.sshMaxFails.OnChanged = func(v float64) {
		maxFailsLabel.SetText(fmt.Sprintf("%d", int(v)))
	}

	d.sshPort = widget.NewEntry()
	d.sshPort.SetText(strconv.Itoa(d.pref.IntWithFallback(prefDefaultSSHPort, defaultSSHPort)))

	policy := d.pref.StringWithFallback(prefKnownHostsPolicy, "ask")
	d.sshPolicy = widget.NewSelect(knownHostsPolicies, nil)
	d.sshPolicy.SetSelected(policy)

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Keepalive 间隔", Widget: container.NewHBox(d.sshKeepalive, keepaliveLabel)},
			{Text: "最大失败次数", Widget: container.NewHBox(d.sshMaxFails, maxFailsLabel)},
			{Text: "默认 SSH 端口", Widget: d.sshPort},
			{Text: "已知主机策略", Widget: d.sshPolicy},
		},
	}

	return container.NewVScroll(form)
}

// buildProxyTab 构建代理设置标签页
func (d *SettingsDialog) buildProxyTab() fyne.CanvasObject {
	d.proxySocks5 = widget.NewEntry()
	d.proxySocks5.SetPlaceHolder("127.0.0.1:1080")
	d.proxySocks5.SetText(d.pref.String(prefGlobalSocks5Proxy))

	d.proxyHTTP = widget.NewEntry()
	d.proxyHTTP.SetPlaceHolder("127.0.0.1:8080")
	d.proxyHTTP.SetText(d.pref.String(prefGlobalHTTPProxy))

	d.proxyBypass = widget.NewMultiLineEntry()
	d.proxyBypass.SetPlaceHolder("localhost,127.0.0.1,*.local")
	d.proxyBypass.SetText(d.pref.String(prefProxyBypassList))
	d.proxyBypass.SetMinRowsVisible(3)

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "SOCKS5 代理", Widget: d.proxySocks5},
			{Text: "HTTP 代理", Widget: d.proxyHTTP},
			{Text: "代理绕过列表", Widget: d.proxyBypass},
		},
	}

	return container.NewVScroll(form)
}

// buildShortcutsTab 构建快捷键标签页（只读表格）
func (d *SettingsDialog) buildShortcutsTab() fyne.CanvasObject {
	cur := d.prefs.Get()
	bindings := make([][2]string, 0, len(shortcutActionOrder))
	for _, action := range shortcutActionOrder {
		key, ok := cur.Shortcuts[action]
		if !ok {
			continue
		}
		bindings = append(bindings, [2]string{key, shortcutActionLabels[action]})
	}
	table := widget.NewTable(
		func() (int, int) { return len(bindings), 2 },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.TableCellID, obj fyne.CanvasObject) {
			label := obj.(*widget.Label)
			if id.Row < 0 || id.Row >= len(bindings) {
				label.SetText("")
				return
			}
			switch id.Col {
			case 0:
				label.SetText(bindings[id.Row][0])
				label.TextStyle = fyne.TextStyle{Monospace: true}
			case 1:
				label.SetText(bindings[id.Row][1])
			}
		},
	)
	table.SetColumnWidth(0, 150)
	table.SetColumnWidth(1, 200)

	header := container.NewHBox(
		widget.NewLabelWithStyle("快捷键", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("功能", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
	)

	return container.NewBorder(header, nil, nil, nil, table)
}

// buildSecurityTab 构建安全设置标签页
func (d *SettingsDialog) buildSecurityTab() fyne.CanvasObject {
	masterEnabled := d.pref.BoolWithFallback(prefMasterPasswordOn, false) || config.IsMasterPasswordEnabled()
	d.securityMasterEnabled = widget.NewCheck("启用主密码保护", nil)
	d.securityMasterEnabled.SetChecked(masterEnabled)

	changePwBtn := widget.NewButton("修改主密码", func() {
		ShowSetMasterPasswordDialog(d.win, d.store, config.IsMasterPasswordEnabled(), nil)
	})

	autoLock := d.pref.IntWithFallback(prefAutoLockTimeout, defaultAutoLockTimeout)
	d.securityAutoLock = widget.NewSlider(1, 60)
	d.securityAutoLock.Value = float64(autoLock)
	autoLockLabel := widget.NewLabel(fmt.Sprintf("%d 分钟", autoLock))
	d.securityAutoLock.OnChanged = func(v float64) {
		autoLockLabel.SetText(fmt.Sprintf("%d 分钟", int(v)))
	}

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "主密码", Widget: d.securityMasterEnabled},
			{Text: "", Widget: changePwBtn},
			{Text: "自动锁定", Widget: container.NewHBox(d.securityAutoLock, autoLockLabel)},
		},
	}

	return container.NewVScroll(form)
}

// save 保存所有设置到 Preferences
func (d *SettingsDialog) save() {
	// 写入声明式 YAML 配置（config.yaml）
	p := *d.prefs.Get()
	p.Font.Size = float32(d.generalFontSize.Value)
	if sb, err := strconv.Atoi(d.generalScrollback.Text); err == nil {
		p.Terminal.ScrollbackLines = sb
	}
	p.Theme.Name = d.generalScheme.Selected
	if err := p.Save(); err != nil {
		dialog.ShowError(fmt.Errorf("保存配置失败: %w", err), d.win)
	}
	d.prefs.Update(&p)

	// 其余设置仍写入 fyne Preferences（尚未迁移到 YAML 的项）
	if cols, err := strconv.Atoi(d.generalCols.Text); err == nil {
		d.pref.SetInt(prefTerminalCols, cols)
	}
	if rows, err := strconv.Atoi(d.generalRows.Text); err == nil {
		d.pref.SetInt(prefTerminalRows, rows)
	}

	d.pref.SetInt(prefKeepaliveInterval, int(d.sshKeepalive.Value))
	d.pref.SetInt(prefMaxKeepaliveFails, int(d.sshMaxFails.Value))
	if port, err := strconv.Atoi(d.sshPort.Text); err == nil {
		d.pref.SetInt(prefDefaultSSHPort, port)
	}
	d.pref.SetString(prefKnownHostsPolicy, d.sshPolicy.Selected)

	d.pref.SetString(prefGlobalSocks5Proxy, d.proxySocks5.Text)
	d.pref.SetString(prefGlobalHTTPProxy, d.proxyHTTP.Text)
	d.pref.SetString(prefProxyBypassList, d.proxyBypass.Text)

	d.pref.SetBool(prefMasterPasswordOn, d.securityMasterEnabled.Checked)
	d.pref.SetInt(prefAutoLockTimeout, int(d.securityAutoLock.Value))
}
