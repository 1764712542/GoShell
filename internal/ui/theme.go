package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// Theme 是 GoShell 的自定义主题，参考 macOS 暗色模式设计语言（iTerm2/Warp 风格）。
// 深色主题：背景 #1e1e2e，前景 #d4d4d4，强调 #0a84ff（macOS 蓝）
// 浅色主题：背景 #f5f5f7，前景 #1d1d1f，强调 #0a84ff（macOS 蓝）
type Theme struct {
	variant fyne.ThemeVariant
	prefs   fyne.Preferences
}

// NewTheme 创建自定义主题
func NewTheme(prefs fyne.Preferences) *Theme {
	t := &Theme{
		variant: theme.VariantDark,
		prefs:   prefs,
	}
	// 从偏好加载主题变体
	if prefs != nil {
		v := prefs.Int("theme_variant")
		if v == 1 {
			t.variant = theme.VariantLight
		}
	}
	return t
}

// Color 实现 fyne.Theme 接口
func (t *Theme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		if variant == theme.VariantDark {
			return color.RGBA{R: 0x1e, G: 0x1e, B: 0x2e, A: 0xff}
		}
		return color.RGBA{R: 0xf5, G: 0xf5, B: 0xf7, A: 0xff}

	case theme.ColorNameForeground:
		if variant == theme.VariantDark {
			return color.RGBA{R: 0xd4, G: 0xd4, B: 0xd4, A: 0xff}
		}
		return color.RGBA{R: 0x1d, G: 0x1d, B: 0x1f, A: 0xff}

	case theme.ColorNamePrimary:
		// macOS 系统蓝
		return color.RGBA{R: 0x0a, G: 0x84, B: 0xff, A: 0xff}

	case theme.ColorNameButton:
		if variant == theme.VariantDark {
			return color.RGBA{R: 0x2d, G: 0x2d, B: 0x3d, A: 0xff}
		}
		return color.RGBA{R: 0xe8, G: 0xe8, B: 0xed, A: 0xff}

	case theme.ColorNameInputBackground:
		if variant == theme.VariantDark {
			return color.RGBA{R: 0x2a, G: 0x2a, B: 0x3a, A: 0xff}
		}
		return color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}

	case theme.ColorNameHeaderBackground:
		if variant == theme.VariantDark {
			return color.RGBA{R: 0x18, G: 0x18, B: 0x25, A: 0xff}
		}
		return color.RGBA{R: 0xec, G: 0xec, B: 0xf0, A: 0xff}

	case theme.ColorNameSeparator:
		if variant == theme.VariantDark {
			return color.RGBA{R: 0x3a, G: 0x3a, B: 0x4a, A: 0xff}
		}
		return color.RGBA{R: 0xd1, G: 0xd1, B: 0xd6, A: 0xff}

	case theme.ColorNameDisabled:
		return color.RGBA{R: 0x66, G: 0x66, B: 0x66, A: 0xff}

	case theme.ColorNamePlaceHolder:
		if variant == theme.VariantDark {
			return color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xff}
		}
		return color.RGBA{R: 0x8e, G: 0x8e, B: 0x93, A: 0xff}

	case theme.ColorNameError:
		// macOS 系统红
		return color.RGBA{R: 0xff, G: 0x45, B: 0x3a, A: 0xff}

	case theme.ColorNameSuccess:
		// macOS 系统绿
		return color.RGBA{R: 0x30, G: 0xd1, B: 0x58, A: 0xff}

	case theme.ColorNameSelection:
		if variant == theme.VariantDark {
			return color.RGBA{R: 0x26, G: 0x4f, B: 0x78, A: 0xff}
		}
		return color.RGBA{R: 0xc2, G: 0xe0, B: 0xff, A: 0xff}

	case theme.ColorNameHover:
		if variant == theme.VariantDark {
			return color.RGBA{R: 0x35, G: 0x35, B: 0x45, A: 0xff}
		}
		return color.RGBA{R: 0xec, G: 0xec, B: 0xf0, A: 0xff}

	case theme.ColorNameShadow:
		if variant == theme.VariantDark {
			return color.RGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x33}
		}
		return color.RGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x1a}
	}
	// 其他颜色使用默认主题
	return theme.DefaultTheme().Color(name, variant)
}

// Font 实现 fyne.Theme 接口，使用默认字体
func (t *Theme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

// Icon 实现 fyne.Theme 接口，使用默认图标
func (t *Theme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

// Size 实现 fyne.Theme 接口
func (t *Theme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return 8 // macOS HIG 推荐的间距
	case theme.SizeNameInnerPadding:
		return 6
	case theme.SizeNameText:
		return 14
	case theme.SizeNameHeadingText:
		return 18
	case theme.SizeNameSubHeadingText:
		return 16
	case theme.SizeNameInputBorder:
		return 2 // 更细的边框
	case theme.SizeNameScrollBar:
		return 8 // 更细的滚动条
	case theme.SizeNameScrollBarSmall:
		return 4
	case theme.SizeNameSeparatorThickness:
		return 1
	}
	return theme.DefaultTheme().Size(name)
}

// SetVariant 设置主题变体（深色/浅色）
func (t *Theme) SetVariant(variant fyne.ThemeVariant) {
	t.variant = variant
	if t.prefs != nil {
		if variant == theme.VariantLight {
			t.prefs.SetInt("theme_variant", 1)
		} else {
			t.prefs.SetInt("theme_variant", 0)
		}
	}
}

// ToggleVariant 切换主题变体
func (t *Theme) ToggleVariant() {
	if t.variant == theme.VariantDark {
		t.SetVariant(theme.VariantLight)
	} else {
		t.SetVariant(theme.VariantDark)
	}
}

// Variant 返回当前主题变体
func (t *Theme) Variant() fyne.ThemeVariant {
	return t.variant
}
