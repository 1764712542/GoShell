package app

// 本文件将事件类型从 internal/event 包重新导出为 app 包的别名，
// 以保持向后兼容（现有代码使用 app.UIEvent 等类型）。
// 事件类型定义已移至 internal/event 包，以打破 app → ssh → app 的循环依赖。
import "github.com/zhuyao/meatshell/internal/event"

// UIEventType 定义后端发送到 UI 的事件类型
type UIEventType = event.UIEventType

const (
	EventTerminal = event.EventTerminal
	EventMonitor  = event.EventMonitor
	EventSFTP     = event.EventSFTP
	EventStatus   = event.EventStatus
	EventTunnel   = event.EventTunnel
	EventProcess  = event.EventProcess
)

// UIEvent 是后端 goroutine 发送给 UI 主线程的统一事件
type UIEvent = event.UIEvent

// TerminalSize 描述终端尺寸
type TerminalSize = event.TerminalSize

// MonitorData 表示一次采样得到的系统指标
type MonitorData = event.MonitorData

// SFTPEntry 表示 SFTP 文件列表中的一项
type SFTPEntry = event.SFTPEntry

// TransferProgress 表示文件传输进度
type TransferProgress = event.TransferProgress

// ConnectionStatus 表示连接状态
type ConnectionStatus = event.ConnectionStatus

const (
	StatusConnecting    = event.StatusConnecting
	StatusConnected     = event.StatusConnected
	StatusDisconnected  = event.StatusDisconnected
	StatusError         = event.StatusError
	StatusHostKeyPrompt = event.StatusHostKeyPrompt
)

// HostKeyInfo 包含主机密钥信息
type HostKeyInfo = event.HostKeyInfo

// TunnelStatus 表示 SSH 隧道状态
type TunnelStatus = event.TunnelStatus

// ProcessEntry 表示一个进程的信息
type ProcessEntry = event.ProcessEntry
