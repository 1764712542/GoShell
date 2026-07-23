// Package worker 定义所有协议 worker 的公共接口。
// 通过接口抽象取代 tab.go 中的 interface{} + type switch 模式，
// 使 Tab 层无需了解具体协议实现即可调用通用方法。
package worker

import "context"

// Worker 是所有协议 worker 的公共接口。
// 取代 tab.go 中的 interface{} + type switch 模式。
type Worker interface {
	Connect(ctx context.Context) error
	Close()
	SendInput(data []byte)
	Resize(cols, rows int) error
	IsConnected() bool
	SessionID() string
}
