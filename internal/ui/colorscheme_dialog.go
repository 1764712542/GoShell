package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// 预览色块数量：背景、前景、红(1)、绿(2)、蓝(4)、品红(5)
const previewSwatchCount = 6

// ColorSchemeDialog 是配色方案选择对话框，列出所有内置配色方案并提供颜色预览。
type ColorSchemeDialog struct {
	win         fyne.Window
	schemes     []ColorScheme
	currentName string
	selected    int
	list        *widget.List
	onApply     func(scheme *ColorScheme)
}

// ShowColorSchemeDialog 创建并显示配色方案选择对话框。
// currentName 为当前使用的配色方案名称；onApply 在用户点击"应用"后回调。
func ShowColorSchemeDialog(win fyne.Window, currentName string, onApply func(scheme *ColorScheme)) {
	d := &ColorSchemeDialog{
		win:         win,
		schemes:     GetColorSchemes(),
		currentName: currentName,
		onApply:     onApply,
	}
	d.show()
}

// show 构建列表并弹出对话框
func (d *ColorSchemeDialog) show() {
	// 定位当前配色方案索引
	selected := 0
	for i, s := range d.schemes {
		if s.Name == d.currentName {
			selected = i
			break
		}
	}
	d.selected = selected

	d.list = widget.NewList(
		func() int { return len(d.schemes) },
		d.itemTemplate,
		d.itemUpdate,
	)
	d.list.OnSelected = func(id widget.ListItemID) {
		d.selected = id
	}
	d.list.Select(selected)

	dialog.ShowCustomConfirm("配色方案", "应用", "取消", d.list, func(confirmed bool) {
		if !confirmed {
			return
		}
		idx := d.selected
		if idx < 0 || idx >= len(d.schemes) {
			return
		}
		scheme := &d.schemes[idx]
		// 持久化选中的配色方案名称
		fyne.CurrentApp().Preferences().SetString("colorScheme", scheme.Name)
		if d.onApply != nil {
			d.onApply(scheme)
		}
	}, d.win)
}

// itemTemplate 构建列表项模板：名称 + 预览色块条
func (d *ColorSchemeDialog) itemTemplate() fyne.CanvasObject {
	nameLabel := widget.NewLabel("配色方案名称")
	nameLabel.TextStyle = fyne.TextStyle{Bold: true}

	swatch := make([]fyne.CanvasObject, previewSwatchCount)
	for i := 0; i < previewSwatchCount; i++ {
		r := canvas.NewRectangle(color.Black)
		r.SetMinSize(fyne.NewSize(28, 16))
		swatch[i] = r
	}
	previewRow := container.NewHBox(swatch...)

	return container.NewVBox(nameLabel, previewRow)
}

// itemUpdate 更新列表项内容
func (d *ColorSchemeDialog) itemUpdate(i widget.ListItemID, obj fyne.CanvasObject) {
	if i < 0 || i >= len(d.schemes) {
		return
	}
	scheme := d.schemes[i]

	vbox := obj.(*fyne.Container)
	nameLabel := vbox.Objects[0].(*widget.Label)
	previewRow := vbox.Objects[1].(*fyne.Container)

	nameLabel.SetText(scheme.Name)

	// 预览顺序：背景、前景、红(1)、绿(2)、蓝(4)、品红(5)
	swatches := previewRow.Objects
	swatches[0].(*canvas.Rectangle).FillColor = scheme.Background
	swatches[1].(*canvas.Rectangle).FillColor = scheme.Foreground
	swatches[2].(*canvas.Rectangle).FillColor = scheme.Colors[1]
	swatches[3].(*canvas.Rectangle).FillColor = scheme.Colors[2]
	swatches[4].(*canvas.Rectangle).FillColor = scheme.Colors[4]
	swatches[5].(*canvas.Rectangle).FillColor = scheme.Colors[5]

	nameLabel.Refresh()
	for _, o := range swatches {
		o.Refresh()
	}
}

// ShowColorSchemeButton 返回一个按钮，点击后打开配色方案选择对话框。
func ShowColorSchemeButton(win fyne.Window, onApply func(scheme *ColorScheme)) *widget.Button {
	return widget.NewButton("配色方案", func() {
		currentName := fyne.CurrentApp().Preferences().String("colorScheme")
		if currentName == "" {
			if def := DefaultColorScheme(); def != nil {
				currentName = def.Name
			}
		}
		ShowColorSchemeDialog(win, currentName, onApply)
	})
}
