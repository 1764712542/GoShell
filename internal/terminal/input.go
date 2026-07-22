package terminal

import (
	"fyne.io/fyne/v2"
)

// keyToANSI 将 Fyne 的 KeyName 转换为对应的 ANSI 转义序列。
// 返回 nil 表示该键没有对应的终端序列（如字母键由 TypedRune 处理）。
func keyToANSI(key fyne.KeyName) []byte {
	switch key {
	// 方向键
	case fyne.KeyUp:
		return []byte{0x1b, '[', 'A'}
	case fyne.KeyDown:
		return []byte{0x1b, '[', 'B'}
	case fyne.KeyRight:
		return []byte{0x1b, '[', 'C'}
	case fyne.KeyLeft:
		return []byte{0x1b, '[', 'D'}
	// Home / End
	case fyne.KeyHome:
		return []byte{0x1b, '[', 'H'}
	case fyne.KeyEnd:
		return []byte{0x1b, '[', 'F'}
	// 翻页
	case fyne.KeyPageUp:
		return []byte{0x1b, '[', '5', '~'}
	case fyne.KeyPageDown:
		return []byte{0x1b, '[', '6', '~'}
	// 删除类
	case fyne.KeyDelete:
		return []byte{0x1b, '[', '3', '~'}
	case fyne.KeyBackspace:
		return []byte{0x7f}
	case fyne.KeyInsert:
		return []byte{0x1b, '[', '2', '~'}
	// 编辑键
	case fyne.KeyTab:
		return []byte{'\t'}
	case fyne.KeyReturn, fyne.KeyEnter:
		return []byte{'\r'}
	case fyne.KeyEscape:
		return []byte{0x1b}
	// 功能键 F1-F12，使用 VT100/VT220 标准序列
	case fyne.KeyF1:
		return []byte{0x1b, 'O', 'P'}
	case fyne.KeyF2:
		return []byte{0x1b, 'O', 'Q'}
	case fyne.KeyF3:
		return []byte{0x1b, 'O', 'R'}
	case fyne.KeyF4:
		return []byte{0x1b, 'O', 'S'}
	case fyne.KeyF5:
		return []byte{0x1b, '[', '1', '5', '~'}
	case fyne.KeyF6:
		return []byte{0x1b, '[', '1', '7', '~'}
	case fyne.KeyF7:
		return []byte{0x1b, '[', '1', '8', '~'}
	case fyne.KeyF8:
		return []byte{0x1b, '[', '1', '9', '~'}
	case fyne.KeyF9:
		return []byte{0x1b, '[', '2', '0', '~'}
	case fyne.KeyF10:
		return []byte{0x1b, '[', '2', '1', '~'}
	case fyne.KeyF11:
		return []byte{0x1b, '[', '2', '3', '~'}
	case fyne.KeyF12:
		return []byte{0x1b, '[', '2', '4', '~'}
	}
	return nil
}

// ctrlKeyToANSI 将 Ctrl+字母 转换为对应的控制字符。
// 例如 Ctrl+A => 0x01, Ctrl+C => 0x03, Ctrl+Z => 0x1a。
// 返回 nil 表示该字母不生成有效控制码（Ctrl+M、Ctrl+[、Ctrl+J、Ctrl+H
// 也会落到对应控制码，调用方应优先处理 keyToANSI 中的 Backspace/Return/Escape/Tab）。
func ctrlKeyToANSI(key fyne.KeyName) []byte {
	if key < fyne.KeyA || key > fyne.KeyZ {
		return nil
	}
	// 字母 A-Z 的 KeyName 为单个字符 "A".."Z"
	c := key[0]
	return []byte{c - 'A' + 1}
}
