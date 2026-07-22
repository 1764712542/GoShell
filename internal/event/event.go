// Package event 定义后端 goroutine 与 UI 主线程之间传递的事件类型。
// 将事件类型独立为此包，避免 internal/app 导入 internal/ssh 等包时产生循环依赖。
package event

// UIEventType 定义后端发送到 UI 的事件类型
type UIEventType int

const (
	EventTerminal UIEventType = iota
	EventMonitor
	EventSFTP
	EventStatus
	EventTunnel
	EventProcess
)

// UIEvent 是后端 goroutine 发送给 UI 主线程的统一事件
type UIEvent struct {
	TabID string
	Type  UIEventType

	// Terminal
	TerminalData []byte // VT100 原始输出
	TerminalSize *TerminalSize

	// Monitor
	Metrics *MonitorData

	// SFTP
	SFTPList     []SFTPEntry
	SFTPProgress *TransferProgress

	// Status
	Status    ConnectionStatus
	StatusMsg string
	HostKey   *HostKeyInfo // 首次连接时的主机密钥确认

	// Tunnel
	TunnelStatus *TunnelStatus

	// Process
	Processes []ProcessEntry
}

// TerminalSize 描述终端尺寸
type TerminalSize struct {
	Cols int
	Rows int
}

// MonitorData 表示一次采样得到的系统指标
type MonitorData struct {
	CPUUsage  float64
	MemTotal  uint64
	MemUsed   uint64
	SwapTotal uint64
	SwapUsed  uint64
	NetSent   uint64 // bytes/s
	NetRecv   uint64
	DiskRead  uint64 // bytes/s
	DiskWrite uint64
}

// SFTPEntry 表示 SFTP 文件列表中的一项
type SFTPEntry struct {
	Name    string
	Size    int64
	IsDir   bool
	ModTime int64 // unix timestamp
}

// TransferProgress 表示文件传输进度
type TransferProgress struct {
	FileName string
	Bytes    int64
	Total    int64
	Speed    float64 // bytes/s
	Done     bool
}

// ConnectionStatus 表示连接状态
type ConnectionStatus int

const (
	StatusConnecting ConnectionStatus = iota
	StatusConnected
	StatusDisconnected
	StatusError
	StatusHostKeyPrompt
)

// HostKeyInfo 包含主机密钥信息
type HostKeyInfo struct {
	Host        string
	Fingerprint string
	KeyType     string
}

// TunnelStatus 表示 SSH 隧道状态
type TunnelStatus struct {
	Type       string // local/remote/dynamic
	LocalAddr  string
	RemoteAddr string
	Active     bool
	Error      string
}

// ProcessEntry 表示一个进程的信息
type ProcessEntry struct {
	PID     int
	User    string
	CPU     float64
	Mem     float64
	Command string
}
