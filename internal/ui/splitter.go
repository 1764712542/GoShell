package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
)

// Splitter 是对 Fyne container.Split 的简单包装。
// Fyne v2 已提供完善的 container.NewVSplit / NewHSplit，此处仅做封装。
type Splitter struct {
	*container.Split
}

// NewVSplitter 创建一个垂直分割容器（上下分割）
func NewVSplitter(top, bottom fyne.CanvasObject) *Splitter {
	return &Splitter{Split: container.NewVSplit(top, bottom)}
}

// NewHSplitter 创建一个水平分割容器（左右分割）
func NewHSplitter(leading, trailing fyne.CanvasObject) *Splitter {
	return &Splitter{Split: container.NewHSplit(leading, trailing)}
}
