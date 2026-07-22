package ui

import "image/color"

// ColorScheme 定义终端配色方案
type ColorScheme struct {
	Name        string
	Background  color.Color
	Foreground  color.Color
	// ANSI 16 色：0-7 标准，8-15 亮色
	Colors [16]color.Color
	// 光标颜色
	Cursor color.Color
	// 选中背景色
	Selection color.Color
}

// hexColor 将 #RRGGBB 转为 color.RGBA
func hexColor(hex string) color.RGBA {
	if len(hex) != 7 || hex[0] != '#' {
		return color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	}
	r := hexToByte(hex[1:3])
	g := hexToByte(hex[3:5])
	b := hexToByte(hex[5:7])
	return color.RGBA{R: r, G: g, B: b, A: 0xff}
}

func hexToByte(s string) uint8 {
	var v uint8
	for i := 0; i < len(s); i++ {
		c := s[i]
		var d uint8
		switch {
		case c >= '0' && c <= '9':
			d = c - '0'
		case c >= 'a' && c <= 'f':
			d = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			d = c - 'A' + 10
		}
		v = v<<4 | d
	}
	return v
}

// hexesToColors 将 16 个 #RRGGBB 字符串转为颜色数组
func hexesToColors(hexes [16]string) [16]color.Color {
	var colors [16]color.Color
	for i, h := range hexes {
		colors[i] = hexColor(h)
	}
	return colors
}

// Built-in 配色方案列表
var colorSchemes = []ColorScheme{
	{
		Name:       "Solarized Dark",
		Background: hexColor("#002b36"),
		Foreground: hexColor("#839496"),
		Cursor:     hexColor("#93a1a1"),
		Selection:  hexColor("#073642"),
		Colors: hexesToColors([16]string{
			"#073642", "#dc322f", "#859900", "#b58900",
			"#268bd2", "#d33682", "#2aa198", "#eee8d5",
			"#002b36", "#cb4b16", "#586e75", "#657b83",
			"#839496", "#6c71c4", "#93a1a1", "#fdf6e3",
		}),
	},
	{
		Name:       "Solarized Light",
		Background: hexColor("#fdf6e3"),
		Foreground: hexColor("#657b83"),
		Cursor:     hexColor("#586e75"),
		Selection:  hexColor("#eee8d5"),
		Colors: hexesToColors([16]string{
			"#073642", "#dc322f", "#859900", "#b58900",
			"#268bd2", "#d33682", "#2aa198", "#eee8d5",
			"#002b36", "#cb4b16", "#586e75", "#657b83",
			"#839496", "#6c71c4", "#93a1a1", "#fdf6e3",
		}),
	},
	{
		Name:       "Dracula",
		Background: hexColor("#282a36"),
		Foreground: hexColor("#f8f8f2"),
		Cursor:     hexColor("#f8f8f2"),
		Selection:  hexColor("#44475a"),
		Colors: hexesToColors([16]string{
			"#000000", "#ff5555", "#50fa7b", "#f1fa8c",
			"#bd93f9", "#ff79c6", "#8be9fd", "#bfbfbf",
			"#4d4d4d", "#ff6e67", "#5af78e", "#f4f991",
			"#caa9fa", "#ff92d0", "#9aedfe", "#e6e6e6",
		}),
	},
	{
		Name:       "Monokai",
		Background: hexColor("#272822"),
		Foreground: hexColor("#f8f8f2"),
		Cursor:     hexColor("#f8f8f0"),
		Selection:  hexColor("#49483e"),
		Colors: hexesToColors([16]string{
			"#272822", "#f92672", "#a6e22e", "#f4bf75",
			"#66d9ef", "#ae81ff", "#a1efe4", "#f8f8f2",
			"#75715e", "#f92672", "#a6e22e", "#f4bf75",
			"#66d9ef", "#ae81ff", "#a1efe4", "#f9f8f5",
		}),
	},
	{
		Name:       "One Dark",
		Background: hexColor("#282c34"),
		Foreground: hexColor("#abb2bf"),
		Cursor:     hexColor("#abb2bf"),
		Selection:  hexColor("#3b4048"),
		Colors: hexesToColors([16]string{
			"#282c34", "#e06c75", "#98c379", "#e5c07b",
			"#61afef", "#c678dd", "#56b6c2", "#abb2bf",
			"#3e4451", "#e06c75", "#98c379", "#e5c07b",
			"#61afef", "#c678dd", "#56b6c2", "#c8ccd4",
		}),
	},
	{
		Name:       "GitHub Dark",
		Background: hexColor("#0d1117"),
		Foreground: hexColor("#c9d1d9"),
		Cursor:     hexColor("#c9d1d9"),
		Selection:  hexColor("#1f6feb"),
		Colors: hexesToColors([16]string{
			"#484f58", "#ff7b72", "#3fb950", "#d29922",
			"#58a6ff", "#bc8cff", "#39c5cf", "#b1bac4",
			"#6e7681", "#ffa198", "#56d364", "#e3b341",
			"#79c0ff", "#d2a8ff", "#56d4dd", "#f0f6fc",
		}),
	},
	{
		Name:       "Nord",
		Background: hexColor("#2e3440"),
		Foreground: hexColor("#d8dee9"),
		Cursor:     hexColor("#d8dee9"),
		Selection:  hexColor("#3b4252"),
		Colors: hexesToColors([16]string{
			"#3b4252", "#bf616a", "#a3be8c", "#ebcb8b",
			"#81a1c1", "#b48ead", "#88c0d0", "#e5e9f0",
			"#4c566a", "#bf616a", "#a3be8c", "#ebcb8b",
			"#81a1c1", "#b48ead", "#8fbcbb", "#eceff4",
		}),
	},
	{
		Name:       "Gruvbox",
		Background: hexColor("#282828"),
		Foreground: hexColor("#ebdbb2"),
		Cursor:     hexColor("#ebdbb2"),
		Selection:  hexColor("#504945"),
		Colors: hexesToColors([16]string{
			"#282828", "#cc241d", "#98971a", "#d79921",
			"#458588", "#b16286", "#689d6a", "#a89984",
			"#928374", "#fb4934", "#b8bb26", "#fabd2f",
			"#83a598", "#d3869b", "#8ec07c", "#ebdbb2",
		}),
	},
	{
		Name:       "Catppuccin Mocha",
		Background: hexColor("#1e1e2e"),
		Foreground: hexColor("#cdd6f4"),
		Cursor:     hexColor("#cdd6f4"),
		Selection:  hexColor("#585b70"),
		Colors: hexesToColors([16]string{
			"#45475a", "#f38ba8", "#a6e3a1", "#f9e2af",
			"#89b4fa", "#f5c2e7", "#94e2d5", "#bac2de",
			"#585b70", "#f38ba8", "#a6e3a1", "#f9e2af",
			"#89b4fa", "#f5c2e7", "#94e2d5", "#a6adc8",
		}),
	},
	{
		Name:       "Tokyo Night",
		Background: hexColor("#1a1b26"),
		Foreground: hexColor("#a9b1d6"),
		Cursor:     hexColor("#c0caf5"),
		Selection:  hexColor("#33467c"),
		Colors: hexesToColors([16]string{
			"#32344a", "#f7768e", "#9ece6a", "#e0af68",
			"#7aa2f7", "#ad8ee6", "#449dab", "#787c99",
			"#444b6a", "#ff7a93", "#b9f27c", "#ff9e64",
			"#7da6ff", "#bb9af7", "#0db9d7", "#acb0d0",
		}),
	},
	{
		Name:       "Ubuntu",
		Background: hexColor("#300a24"),
		Foreground: hexColor("#eeeeec"),
		Cursor:     hexColor("#ffffff"),
		Selection:  hexColor("#5c3566"),
		Colors: hexesToColors([16]string{
			"#2e3436", "#cc0000", "#4e9a06", "#c4a000",
			"#3465a4", "#75507b", "#06989a", "#d3d7cf",
			"#555753", "#ef2929", "#8ae234", "#fce94f",
			"#729fcf", "#ad7fa8", "#34e2e2", "#eeeeec",
		}),
	},
}

// GetColorSchemes 返回所有内置配色方案
func GetColorSchemes() []ColorScheme {
	return colorSchemes
}

// FindColorScheme 按名称查找配色方案
func FindColorScheme(name string) *ColorScheme {
	for i := range colorSchemes {
		if colorSchemes[i].Name == name {
			return &colorSchemes[i]
		}
	}
	return nil
}

// DefaultColorScheme 返回默认配色方案（Dracula）
func DefaultColorScheme() *ColorScheme {
	return FindColorScheme("Dracula")
}
