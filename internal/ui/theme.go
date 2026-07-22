package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// Theme 是 Meatshell 的自定义主题，支持深色和浅色两种变体。
// 深色主题：背景 #1e1e2e，前景 #cdd6f4，强调 #89b4fa
// 浅色主题：背景 #eff1f5，前景 #4c4f69，强调 #1e66f5
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
		return color.RGBA{R: 0xef, G: 0xf1, B: 0xf5, A: 0xff}

	case theme.ColorNameForeground:
		if variant == theme.VariantDark {
			return color.RGBA{R: 0xcd, G: 0xd6, B: 0xf4, A: 0xff}
		}
		return color.RGBA{R: 0x4c, G: 0x4f, B: 0x69, A: 0xff}

	case theme.ColorNamePrimary:
		if variant == theme.VariantDark {
			return color.RGBA{R: 0x89, G: 0xb4, B: 0xfa, A: 0xff}
		}
		return color.RGBA{R: 0x1e, G: 0x66, B: 0xf5, A: 0xff}

	case theme.ColorNameButton:
		if variant == theme.VariantDark {
			return color.RGBA{R: 0x31, G: 0x31, B: 0x44, A: 0xff}
		}
		return color.RGBA{R: 0xdc, G: 0xe0, B: 0xe8, A: 0xff}

	case theme.ColorNameInputBackground:
		if variant == theme.VariantDark {
			return color.RGBA{R: 0x31, G: 0x31, B: 0x44, A: 0xff}
		}
		return color.RGBA{R: 0xe6, G: 0xe9, B: 0xef, A: 0xff}

	case theme.ColorNameHeaderBackground:
		if variant == theme.VariantDark {
			return color.RGBA{R: 0x18, G: 0x18, B: 0x25, A: 0xff}
		}
		return color.RGBA{R: 0xd2, G: 0xd9, B: 0xe0, A: 0xff}

	case theme.ColorNameSeparator:
		if variant == theme.VariantDark {
			return color.RGBA{R: 0x45, G: 0x47, B: 0x5a, A: 0xff}
		}
		return color.RGBA{R: 0xac, G: 0xbe, B: 0xe0, A: 0xff}
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
	case theme.SizeNameText:
		return 14
	case theme.SizeNameHeadingText:
		return 18
	case theme.SizeNameSubHeadingText:
		return 16
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
