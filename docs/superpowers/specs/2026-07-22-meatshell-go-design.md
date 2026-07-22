# Meatshell-Go 设计文档

> 将 Rust + Slint 实现的 [meatshell](https://github.com/jeff141/meatshell) SSH/终端客户端用 Go + Fyne 全量重构，打包为 macOS / Windows / Linux 三平台可用的原生应用。

## 1. 目标与范围

### 1.1 项目目标
- **功能对等**：实现 meatshell 的全部 16+ 功能模块
- **性能优先**：纯 Go 编译，单二进制，低内存占用（目标 < 50MB 常驻）
- **跨平台**：macOS（arm64/amd64）、Windows（amd64）、Linux（amd64/arm64）统一代码库
- **个人学习**：理解 SSH 协议、终端模拟、系统监控、GUI 架构的工程实践

### 1.2 功能清单（全量）

| # | 功能模块 | 优先级 | 说明 |
|---|---------|--------|------|
| 1 | GUI 框架 + 主题系统 | P0 | 深色/浅色/跟随系统，FinalShell 风格布局 |
| 2 | 多标签页界面 | P0 | 欢迎页 + 多个会话标签，动态创建/关闭 |
| 3 | SSH 客户端 | P0 | 密码/私钥/加密私钥/keyboard-interactive 认证 |
| 4 | VT/ANSI 终端模拟 | P0 | 支持 htop/vim/btop 全屏渲染 |
| 5 | 会话管理 | P0 | JSON 持久化，增删改查，分组，导入导出 |
| 6 | 本机系统监控 | P0 | CPU/内存/交换/网络/磁盘 + Sparkline |
| 7 | 远端系统监控 | P1 | 通过 SSH 采集远端指标 |
| 8 | 远端进程监控 | P1 | 按 CPU 排序的只读进程表 |
| 9 | SFTP 文件浏览 | P1 | 上传/下载/拖拽/目录树 |
| 10 | SSH 隧道 | P1 | 本地 -L / 远程 -R / 动态 -D SOCKS5 |
| 11 | 快捷命令 + 群发 | P1 | 命令输入框 + 历史 + 广播到所有会话 |
| 12 | 出站代理 | P2 | SOCKS5 / HTTP CONNECT |
| 13 | 串口会话 | P2 | COM / /dev/ttyUSB 串口连接 |
| 14 | Telnet 会话 | P2 | Telnet 协议客户端 |
| 15 | 密码加密存储 | P0 | ChaCha20-Poly1305 加密会话密码 |
| 16 | known_hosts 校验 | P0 | 首次确认 + 密钥变化告警 |
| 17 | 导入 ~/.ssh/config | P1 | 解析 OpenSSH 配置文件 |
| 18 | ZMODEM 接收 | P2 | 终端内 `sz` 文件接收 |
| 19 | 终端分屏 | P2 | 水平/垂直分割 |
| 20 | 多语言 i18n | P2 | 中/英文切换 |
| 21 | 更新检查 | P2 | GitHub Releases API 检查新版本 |

### 1.3 非目标
- 不实现 SSH 服务端功能
- 不实现 SSH agent 转发
- 不实现 X11 转发
- 不做代码签名/公证（个人项目，用户手动去隔离属性）

## 2. 技术栈

### 2.1 依赖清单

| 用途 | Go 包 | 版本 |
|------|-------|------|
| GUI 框架 | `fyne.io/v2` | latest |
| SSH 协议 | `golang.org/x/crypto/ssh` | latest |
| SFTP | `github.com/pkg/sftp` | latest |
| 系统指标 | `github.com/shirou/gopsutil/v3` | latest |
| VT100 终端 | `github.com/zyedidia/vt100` | latest |
| 串口 | `github.com/bugst/go-serial` | latest |
| 本地 PTY | `github.com/creack/pty` | latest |
| SOCKS5 代理 | `golang.org/x/net/proxy` | latest |
| UUID | `github.com/google/uuid` | latest |
| 日志 | `log/slog`（标准库） | Go 1.21+ |
| 加密 | `golang.org/x/crypto/chacha20poly1305` | latest |
| Argon2 | `golang.org/x/crypto/argon2` | latest |
| Unicode 宽度 | `github.com/rivo/uniseg` | latest |

### 2.2 标准库使用
- `encoding/json` — 序列化
- `crypto/aes`, `crypto/hmac`, `crypto/sha1`, `crypto/sha256` — PPK 解密
- `net/http` — 更新检查
- `image` — 壁纸解码
- `regexp` — SSH config 解析
- `os`, `path/filepath` — 配置目录
- `time`, `context`, `sync` — 并发控制

### 2.3 Go 版本
- 最低 Go 1.22（泛型 + slog + log/slog）

## 3. 架构设计

### 3.1 分层架构

```
┌─────────────────────────────────────────────────────┐
│                    UI 层 (Fyne)                      │
│  Window → Tabs → [Welcome | SessionView]            │
│  SessionView = Sidebar + Terminal + SFTP + QuickCmd │
├─────────────────────────────────────────────────────┤
│                  应用编排层 (app)                     │
│  App struct: 管理标签页、会话生命周期、事件分发       │
├─────────────────────────────────────────────────────┤
│                    后端服务层                         │
│  ┌──────┐ ┌──────┐ ┌────────┐ ┌───────┐ ┌────────┐│
│  │ SSH  │ │ SFTP │ │Monitor │ │Serial │ │Telnet  ││
│  │Worker│ │Worker│ │Worker  │ │Worker │ │Worker  ││
│  └──────┘ └──────┘ └────────┘ └───────┘ └────────┘│
├─────────────────────────────────────────────────────┤
│                    基础设施层                         │
│  Config(持久化) | Proxy(代理) | i18n | Log | Update │
└─────────────────────────────────────────────────────┘
```

### 3.2 并发模型

**核心原则：**
- Fyne UI 只在主 goroutine 操作（Fyne 线程安全约束，类似 Slint）
- 所有后端工作在独立 goroutine 中运行
- 后端 → UI：通过 `chan UIEvent` 发送，主线程消费并刷新
- UI → 后端：通过方法调用 + context 取消

**UIEvent 类型：**
```go
type UIEvent struct {
    TabID    string
    Type     UIEventType
    Terminal *TerminalUpdate  // 终端输出
    Monitor  *MonitorUpdate   // 监控数据
    SFTP     *SFTPUpdate      // 文件列表/传输进度
    Status   *StatusUpdate     // 连接状态/错误
    Tunnel   *TunnelUpdate     // 隧道状态
}

type UIEventType int
const (
    EventTerminal UIEventType = iota
    EventMonitor
    EventSFTP
    EventStatus
    EventTunnel
)
```

**SSH Worker 生命周期：**
```
用户点击连接
  → App.CreateSession(session)
    → ssh.NewWorker(session, uiChan)
      → worker.Connect()  // goroutine
        → ssh.Dial + Auth
        → ssh.Session + Shell
        → 读循环: stdout → vt100 → UIEvent
        → 写循环: 接收键盘输入 → stdin
      → worker.Monitor()  // goroutine
        → 定时采集远端指标 → UIEvent
  → App 添加新 Tab，渲染终端 widget
```

### 3.3 目录结构

```
meatshell-go/
├── cmd/
│   └── meatshell/
│       └── main.go                 # 入口：初始化 app、加载配置、启动 UI
├── internal/
│   ├── app/
│   │   ├── app.go                  # App struct：管理所有标签页和会话
│   │   ├── tab.go                  # Tab 生命周期管理
│   │   └── event.go               # UIEvent 定义和分发
│   ├── config/
│   │   ├── session.go              # Session 数据模型
│   │   ├── store.go                # JSON 加载/保存、会话 CRUD
│   │   ├── crypto.go               # ChaCha20-Poly1305 加密
│   │   └── sshconfig.go            # ~/.ssh/config 解析导入
│   ├── ssh/
│   │   ├── client.go               # SSH 连接管理（Dial/Close）
│   │   ├── auth.go                 # 认证方法（password/key/PPK/keyboard-interactive）
│   │   ├── worker.go               # 会话 worker（读写循环、生命周期）
│   │   ├── knownhosts.go           # known_hosts 校验逻辑
│   │   ├── tunnel.go               # 端口转发（-L/-R/-D）
│   │   ├── ppk.go                  # PuTTY PPK v2/v3 密钥解析
│   │   └── proxy.go                # 通过代理建立 SSH 连接
│   ├── sftp/
│   │   ├── client.go               # SFTP 客户端封装
│   │   ├── browser.go              # 远端目录浏览模型
│   │   └── transfer.go              # 上传/下载（带进度）
│   ├── terminal/
│   │   ├── emulator.go             # vt100 包装 + 网格管理
│   │   ├── widget.go               # Fyne Widget 实现（自定义渲染）
│   │   ├── input.go                # 键盘事件处理 → 写入 SSH
│   │   ├── render.go               # 脏行追踪 + Canvas 绘制
│   │   └── zmodem.go               # ZMODEM sz 接收
│   ├── monitor/
│   │   ├── local.go                # 本机指标采集（gopsutil）
│   │   ├── remote.go               # 远端指标采集（SSH 命令）
│   │   ├── process.go              # 远端进程列表
│   │   └── types.go                # Metrics 结构体
│   ├── proxy/
│   │   ├── socks5.go               # SOCKS5 代理拨号
│   │   └── http.go                 # HTTP CONNECT 代理
│   ├── serial/
│   │   └── serial.go               # 串口会话 worker
│   ├── telnet/
│   │   └── telnet.go               # Telnet 客户端
│   ├── ui/
│   │   ├── theme.go                # 深色/浅色主题 + 设计 tokens
│   │   ├── window.go               # 主窗口构建
│   │   ├── sidebar.go              # 左侧系统监控面板
│   │   ├── tabs.go                 # 顶部标签栏
│   │   ├── welcome.go              # 欢迎页/快速连接
│   │   ├── session_dialog.go        # 新建/编辑会话弹框
│   │   ├── terminal_view.go         # 终端视图容器
│   │   ├── sftp_view.go             # SFTP 文件浏览面板
│   │   ├── tunnel_panel.go          # SSH 隧道面板
│   │   ├── quickcmd.go              # 快捷命令栏
│   │   ├── sparkline.go             # 迷你折线图 widget
│   │   ├── splitter.go              # 可拖拽分割容器
│   │   └── i18n_label.go            # 国际化文本组件
│   ├── i18n/
│   │   ├── i18n.go                  # 翻译加载和查询
│   │   └── strings.go               # 键值常量
│   ├── update/
│   │   └── checker.go               # GitHub Releases 检查
│   └── log/
│       └── log.go                   # slog 初始化和封装
├── assets/
│   ├── icon.png                    # 应用图标
│   ├── icon.ico                     # Windows 图标
│   ├── icon.icns                    # macOS 图标
│   └── wallpaper.jpg                # 默认壁纸
├── lang/
│   ├── zh-CN.json                   # 中文
│   └── en-US.json                   # 英文
├── packaging/
│   ├── darwin/
│   │   ├── Info.plist               # macOS bundle 配置
│   │   └── build.sh                 # .app 打包脚本
│   ├── windows/
│   │   └── build.ps1                # Windows 打包脚本
│   └── linux/
│       ├── meatshell.desktop        # 桌面入口
│       └── install.sh               # 安装脚本
├── go.mod
├── go.sum
├── Makefile                         # 构建目标
├── .goreleaser.yml                  # 发布自动化
└── docs/
    └── superpowers/
        └── specs/
            └── 2026-07-22-meatshell-go-design.md  # 本文档
```

## 4. 模块详细设计

### 4.1 配置与会话管理 (config)

**Session 数据模型：**
```go
type Session struct {
    ID          string    `json:"id"`
    Name        string    `json:"name"`
    Group       string    `json:"group,omitempty"`
    Type        SessionType `json:"type"`          // ssh/serial/telnet
    Host        string    `json:"host"`
    Port        int       `json:"port"`
    Username    string    `json:"username"`
    Password    string    `json:"password"`         // 加密存储
    PrivateKey  string    `json:"private_key,omitempty"` // 加密存储
    Passphrase  string    `json:"passphrase,omitempty"`   // 加密存储
    AuthMethod  string    `json:"auth_method"`      // password/key/agent
    Proxy       *ProxyConfig `json:"proxy,omitempty"`
    Tunnels     []TunnelConfig `json:"tunnels,omitempty"`
    QuickCommands []string `json:"quick_commands,omitempty"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}

type SessionType string
const (
    SessionSSH    SessionType = "ssh"
    SessionSerial SessionType = "serial"
    SessionTelnet SessionType = "telnet"
)
```

**配置目录：**
- Windows: `%APPDATA%/meatshell/sessions.json`
- Linux: `~/.config/meatshell/sessions.json`
- macOS: `~/Library/Application Support/meatshell/sessions.json`
- 使用 `os.UserConfigDir()` 获取

**密码加密方案：**
- 算法：ChaCha20-Poly1305 AEAD
- 密钥派生：从机器特征（主机名 + 用户名 + 硬盘序列号）通过 SHA-256 派生 32 字节密钥
- 密文格式：base64(nonce + ciphertext + tag)
- 注意：这是本地保护，非强加密（密钥在本机可推导）

**~/.ssh/config 导入：**
- 解析 `Host`、`HostName`、`User`、`Port`、`IdentityFile`、`ProxyJump` 字段
- 支持 `Host *` 通配符匹配
- 导入后创建 Session 记录，但不存储密码

### 4.2 SSH 客户端 (ssh)

**连接流程：**
```
ssh.Dial(addr, config)
  → 认证（password / publickey / keyboard-interactive）
  → 创建 Session
  → 请求 PTY（xterm-256color, 80x24）
  → 请求 Shell
  → 启动读写循环
```

**认证方法：**
```go
func (w *Worker) authMethods() ([]ssh.AuthMethod, error) {
    var methods []ssh.AuthMethod
    // 1. 密码认证
    if w.session.Password != "" {
        methods = append(methods, ssh.Password(w.session.Password))
    }
    // 2. 私钥认证（OpenSSH PEM / PPK）
    if w.session.PrivateKey != "" {
        signer, err := parseSigner(w.session.PrivateKey, w.session.Passphrase)
        if err == nil {
            methods = append(methods, ssh.PublicKeys(signer))
        }
    }
    // 3. keyboard-interactive（交互式输入）
    methods = append(methods, ssh.KeyboardInteractive(w.interactiveCB))
    return methods, nil
}
```

**known_hosts 校验：**
- 首次连接：弹窗显示主机指纹，用户确认后写入 `known_hosts` 文件
- 后续连接：比对指纹，不匹配则拒绝并提示
- `known_hosts` 路径：`~/.ssh/known_hosts`

**PuTTY PPK 解析：**
- PPK v2：AES-256-CBC 解密 + HMAC-SHA1 验证
- PPK v3：Argon2 密钥派生 + AES-256-CTR + HMAC-SHA-256
- 解密后转换为 OpenSSH PEM 格式，再用 `ssh.ParsePrivateKey` 解析

**端口转发：**
```go
// 本地转发 -L
func (w *Worker) LocalForward(localAddr, remoteAddr string) error

// 远程转发 -R
func (w *Worker) RemoteForward(remoteAddr, localAddr string) error

// 动态转发 -D (SOCKS5)
func (w *Worker) DynamicForward(localAddr string) error
    // 启动 SOCKS5 服务器，通过 SSH 连接转发
```

### 4.3 终端模拟 (terminal)

**VT100 集成：**
```go
type Emulator struct {
    vt      *vt100.VT100
    rows    int
    cols    int
    dirty   map[int]bool  // 脏行索引
    mu      sync.Mutex
}

func (e *Emulator) Write(data []byte) {
    e.mu.Lock()
    defer e.mu.Unlock()
    e.vt.Write(data)  // vt100 库解析 ANSI 并更新网格
    // vt100 库会标记脏行
}

func (e *Emulator) DirtyLines() []int {
    // 返回需要重绘的行号
}

func (e *Emulator) Cell(row, col int) vt100.Cell {
    // 返回指定位置的字符和样式
}
```

**Fyne Widget 实现：**
```go
type TerminalWidget struct {
    widget.BaseWidget
    emulator  *Emulator
    inputChan chan []byte  // 键盘输入发送到 SSH
    fontSize  float32
    cols, rows int
}

func (t *TerminalWidget) CreateRenderer() fyne.WidgetRenderer {
    // 返回自定义渲染器
    // 使用 canvas.Text 逐 cell 绘制
    // 脏行追踪：只重绘变化的行
}

func (t *TerminalWidget) TypedKey(key *fyne.KeyEvent) {
    // 键盘事件 → ANSI 转义序列 → inputChan
}

func (t *TerminalWidget) TypedRune(r rune) {
    // 可见字符 → inputChan
}
```

**渲染优化：**
- 脏行追踪：vt100 库报告已变化的行，只重绘这些行
- 双缓冲：先绘制到离屏 canvas，再一次性刷新
- 等宽字体：使用 Fyne 的 `monospace` 字体，确保对齐
- 高吞吐：批量处理（16ms 内的多次输出合并为一次刷新，约 60fps）

**键盘映射：**
- 方向键 → ANSI 转义序列（`\x1b[A` 等）
- Ctrl+C → `\x03`
- Ctrl+D → `\x04`
- Tab → `\t`
- Backspace → `\x7f`
- Enter → `\r`

**ZMODEM 接收：**
- 检测 stdout 中的 ZMODEM 起始序列 `**\x18B00000000000000`
- 进入 ZMODEM 模式，解析文件名和大小
- 写入本地文件系统，完成后恢复终端模式

### 4.4 系统监控 (monitor)

**本机指标采集（gopsutil）：**
```go
type Metrics struct {
    CPUUsage    float64   // 0-100%
    MemTotal    uint64    // bytes
    MemUsed     uint64
    SwapTotal   uint64
    SwapUsed    uint64
    NetSent     uint64    // bytes/s
    NetRecv     uint64
    DiskRead    uint64    // bytes/s
    DiskWrite   uint64
    Processes   []ProcessInfo
}
```

- 采样间隔：1 秒
- CPU：两次采样间的差值计算使用率
- 网络：网卡速率 = (当前计数 - 上次计数) / 间隔
- Sparkline：保留最近 60 个采样点

**远端指标采集：**
- 通过 SSH 执行命令，解析输出：
  - CPU：`cat /proc/stat | grep '^cpu '`
  - 内存：`free -b` 或 `cat /proc/meminfo`
  - 磁盘：`df -B1`
  - 网络：`cat /proc/net/dev`
  - 进程：`ps aux --sort=-%cpu | head -20`
- 兼容 macOS 远端：使用 `top -l 1`、`vm_stat`、`df` 等
- 采样间隔：2 秒（减少远端负载）

### 4.5 SFTP 文件浏览 (sftp)

**操作：**
- `List(path) → []FileInfo`
- `Upload(localPath, remotePath, progressChan)`
- `Download(remotePath, localPath, progressChan)`
- `Mkdir(path)`, `Remove(path)`, `Rename(old, new)`

**UI 集成：**
- 树形目录浏览器，双击进入目录
- 拖拽上传：监听 Fyne `DropEvent`，遍历拖入的文件路径
- 下载右键菜单：选择本地保存位置（Fyne 文件对话框）
- 传输进度条：实时显示速度和百分比

### 4.6 UI 组件 (ui)

**主窗口布局：**
```
┌────────────────────────────────────────────┐
│  [+] [Tab1] [Tab2] [Tab3]        [×]      │ ← 标签栏
├────────┬───────────────────────────────────┤
│        │                                   │
│ 本机   │       终端 / SFTP / 监控          │
│ 监控   │                                   │
│ 侧栏   │                                   │
│        │                                   │
│ CPU    │                                   │
│ ▁▂▄▆█ │                                   │
│        │                                   │
│ MEM    │                                   │
│ ▁▂▃▅▇ │                                   │
│        │                                   │
├────────┴───────────────────────────────────┤
│  > 输入命令...            [群发] [历史]    │ ← 快捷命令栏
└────────────────────────────────────────────┘
```

**Sparkline Widget：**
```go
type Sparkline struct {
    widget.BaseWidget
    values []float64
    color  color.Color
    max    int  // 最大数据点数（60）
}

func (s *Sparkline) Push(val float64) {
    // 滑动窗口，保留最近 max 个值
}
```

**主题系统：**
- 基于 Fyne `Theme` 接口实现自定义主题
- 设计 tokens：背景色、前景色、强调色、终端配色（16 色 ANSI）
- 跟随系统：监听 `app.Settings().Theme()` 变化
- 用户偏好持久化到 `app.Preferences()`

**终端配色（ANSI 16 色）：**
```go
var DarkTermColors = [16]color.Color{
    color.Black,           // 0 黑
    {R: 0xCC, G: 0x00, B: 0x00},  // 1 红
    {R: 0x4E, G: 0x9A, B: 0x06},  // 2 绿
    {R: 0xC4, G: 0xA0, B: 0x00},  // 3 黄
    {R: 0x34, G: 0x65, A: 0xA4},  // 4 蓝
    {R: 0x75, G: 0x50, B: 0x7B},  // 5 紫
    {R: 0x06, G: 0x98, B: 0x9A},  // 6 青
    {R: 0xD3, G: 0xD7, B: 0xCF},  // 7 白
    // 亮色 8-15...
}
```

### 4.7 代理 (proxy)

**SOCKS5：**
```go
func socks5Dialer(proxyAddr, targetAddr string) (net.Conn, error) {
    auth := &proxy.Auth{User: user, Password: pass}
    dialer, err := proxy.SOCKS5("tcp", proxyAddr, auth, &net.Dialer{})
    return dialer.Dial("tcp", targetAddr)
}
```

**HTTP CONNECT：**
```go
func httpConnectDialer(proxyAddr, targetAddr string) (net.Conn, error) {
    conn, _ := net.Dial("tcp", proxyAddr)
    fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\n\r\n", targetAddr)
    // 读取响应，检查 200
    return conn, nil
}
```

**SSH over Proxy：**
- 将代理拨号器传入 `ssh.Dial` 的 `net.Conn` 参数

## 5. 跨平台打包

### 5.1 GoReleaser 配置

```yaml
# .goreleaser.yml
project_name: meatshell
builds:
  - id: meatshell
    main: ./cmd/meatshell
    binary: meatshell
    env:
      - CGO_ENABLED=1  # Fyne 需要 CGO（OpenGL）
    goos:
      - darwin
      - windows
      - linux
    goarch:
      - amd64
      - arm64
    ignore:
      - goos: windows
        goarch: arm64
```

### 5.2 各平台打包

**macOS：**
```bash
# 方法 1: fyne package
fyne package -os darwin -icon assets/icon.png -name meatshell
# 生成 meatshell.app

# 方法 2: 手动构建 universal binary
CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 go build -o meatshell-amd64
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -o meatshell-arm64
lipo -create -output meatshell meatshell-amd64 meatshell-arm64
# 组装 .app bundle
```

**Windows：**
```bash
# 交叉编译（需 mingw-w64）
CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc CXX=x86_64-w64-mingw32-g++ \
  GOOS=windows GOARCH=amd64 go build -o meatshell.exe

# 或用 fyne package
fyne package -os windows -icon assets/icon.ico -name meatshell
# 生成 meatshell.exe（含图标）
```

**Linux：**
```bash
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o meatshell
# 或
fyne package -os linux -icon assets/icon.png -name meatshell
# 生成 tar.gz + .desktop
```

### 5.3 Makefile

```makefile
.PHONY: build run clean package-all package-mac package-windows package-linux

APP_NAME := meatshell
MAIN := ./cmd/meatshell

build:
	go build -o $(APP_NAME) $(MAIN)

run: build
	./$(APP_NAME)

package-mac:
	fyne package -os darwin -icon assets/icon.png -name $(APP_NAME)

package-windows:
	fyne package -os windows -icon assets/icon.ico -name $(APP_NAME)

package-linux:
	fyne package -os linux -icon assets/icon.png -name $(APP_NAME)

package-all: package-mac package-windows package-linux

clean:
	rm -f $(APP_NAME) *.app *.exe *.tar.gz
```

## 6. 错误处理

- Go 风格：`error` 接口，`fmt.Errorf("xxx: %w", err)` 包装
- 自定义错误类型：`var ErrHostKeyMismatch = errors.New("host key mismatch")`
- 用户友好错误：SSH 连接失败时弹窗显示可读信息（中文）
- 日志：`slog.Error("ssh connect failed", "host", host, "err", err)`

## 7. 测试策略

- **单元测试**：config 序列化、PPK 解析、known_hosts 比对、ANSI 解析
- **集成测试**：使用本地 SSH 服务器（`testdata/sshd`）测试连接/认证/Shell
- **手动测试**：终端渲染（htop/vim）、SFTP 拖拽、多标签并发

## 8. 开发顺序

按依赖关系分阶段实现：

1. **基础设施**：go.mod、log、config（会话模型+JSON 持久化）
2. **SSH 核心**：ssh client（连接+认证）、terminal emulator（vt100+Fyne 渲染）
3. **UI 框架**：window、tabs、terminal_view、sidebar（监控）
4. **会话管理**：session_dialog、store CRUD、known_hosts
5. **SFTP**：sftp client + sftp_view
6. **监控**：local/remote monitor + sparkline
7. **隧道**：tunnel + tunnel_panel
8. **增强功能**：proxy、serial、telnet、zmodem、quickcmd、i18n
9. **打包**：Makefile、goreleaser、各平台脚本
