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

// shortcutBindings 列出快捷键绑定（只读展示）
var shortcutBindings = [][2]string{
	{"Ctrl+N", "新建会话"},
	{"Ctrl+W", "关闭标签页"},
	{"Ctrl+T", "切换主题"},
	{"Ctrl+Shift+B", "同步输入模式"},
	{"Ctrl+Shift+L", "终端日志"},
	{"Ctrl+Shift+F", "搜索"},
	{"Ctrl+Shift+C", "复制"},
	{"Ctrl+Shift+V", "粘贴"},
}

// ShowSettingsDialog 显示全局设置对话框
func ShowSettingsDialog(win fyne.Window, store *config.Store) {
	d := &SettingsDialog{
		win:   win,
		pref:  fyne.CurrentApp().Preferences(),
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
	fontSize := d.pref.FloatWithFallback(prefFontSize, defaultFontSize)
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
	currentScheme := d.pref.String(prefColorScheme)
	if currentScheme == "" {
		if def := DefaultColorScheme(); def != nil {
			currentScheme = def.Name
		}
	}
	d.generalScheme = widget.NewSelect(schemeNames, nil)
	d.generalScheme.SetSelected(currentScheme)

	d.generalScrollback = widget.NewEntry()
	d.generalScrollback.SetText(strconv.Itoa(d.pref.IntWithFallback(prefScrollbackSize, defaultScrollbackSize)))

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
	table := widget.NewTable(
		func() (int, int) { return len(shortcutBindings), 2 },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.TableCellID, obj fyne.CanvasObject) {
			label := obj.(*widget.Label)
			if id.Row < 0 || id.Row >= len(shortcutBindings) {
				label.SetText("")
				return
			}
			switch id.Col {
			case 0:
				label.SetText(shortcutBindings[id.Row][0])
				label.TextStyle = fyne.TextStyle{Monospace: true}
			case 1:
				label.SetText(shortcutBindings[id.Row][1])
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
	d.pref.SetFloat(prefFontSize, d.generalFontSize.Value)
	if cols, err := strconv.Atoi(d.generalCols.Text); err == nil {
		d.pref.SetInt(prefTerminalCols, cols)
	}
	if rows, err := strconv.Atoi(d.generalRows.Text); err == nil {
		d.pref.SetInt(prefTerminalRows, rows)
	}
	d.pref.SetString(prefColorScheme, d.generalScheme.Selected)
	if sb, err := strconv.Atoi(d.generalScrollback.Text); err == nil {
		d.pref.SetInt(prefScrollbackSize, sb)
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
