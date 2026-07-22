# GoShell 全功能增强实现计划

## 执行策略
分 5 个并行批次执行，每批次内的任务相互独立。批次间有依赖关系需顺序执行。

## 批次 1：数据层扩展（无依赖，可并行）

### Task 1.1: Session 增加 Group 字段
- 文件：`internal/config/session.go`
- 已有 `Group string` 字段（从搜索结果确认），但 Store 的 CRUD 可能未暴露分组操作
- 检查 `internal/config/store.go`，增加 `ListGroups() []string`、`MoveToGroup(id, group string) error`
- 验证：`go build ./internal/config/`

### Task 1.2: QuickCmd 增加 Group 字段
- 文件：`internal/ui/quickcmd.go`
- 当前 QuickCmdBar 只有 `entry`、`history`，无 QuickCmd 结构体
- 新建 `internal/config/quickcmd.go`：定义 `QuickCmd struct{Name, Command, Group string}`
- Store 增加 `ListQuickCmds() []QuickCmd`、`SaveQuickCmd(Question QuickCmd)`、`DeleteQuickCmd(name string)`
- 验证：`go build ./internal/config/`

### Task 1.3: 配色方案数据
- 新建 `internal/ui/colorschemes.go`
- 定义 `ColorScheme struct{Name, Background, Foreground string, Colors [16]string}`
- 内置 10+ 预设：Solarized Dark/Light, Dracula, Monokai, One Dark, GitHub, Nord, Gruvbox, Catppuccin, Tokyo Night
- 验证：`go build ./internal/ui/`

## 批次 2：核心 UI 功能（依赖批次 1）

### Task 2.1: 会话分组与搜索
- 文件：`internal/ui/welcome.go`
- 修改 `buildSessionList`：按 Group 分组渲染，组标题可折叠
- 顶部增加搜索框 `*widget.Entry`，OnChanged 实时过滤
- 增加右键菜单：使用 `widget.NewPopUpMenu` 或自定义 Popup
- 右键选项：连接/编辑/复制/移动到分组/删除
- 验证：`go build ./internal/ui/`

### Task 2.2: 标签页增强
- 文件：`internal/ui/tabs.go`
- TabItem 增加 `Color string` 字段
- tabButton 增加颜色指示器（左侧 3px 竖条）
- 实现 `fyne.Tappable` 的 `TappedSecondary`（右键）显示菜单
- 右键菜单：关闭/关闭其他/关闭右侧/复制会话/重命名/设置颜色
- 标签拖拽：实现 `fyne.Draggable` 接口
- 验证：`go build ./internal/ui/`

### Task 2.3: 终端日志记录
- 新建 `internal/terminal/logger.go`
- `TerminalLogger struct{file *os.File, enabled bool}`
- 方法：`Start(filepath string) error`、`Stop()`、`Write(data []byte)`
- 文件：`internal/ui/terminal_view.go` 工具栏增加"记录"按钮
- 验证：`go build ./...`

### Task 2.4: 断线自动重连
- 文件：`internal/ssh/worker.go`
- 增加 `reconnectAttempts int`、`maxReconnect int`、`reconnectDelay time.Duration`
- `readLoop` 检测断开后，如果未主动关闭，触发重连
- 新增 `reconnect(ctx) error` 方法
- Tab 层处理 StatusDisconnected 事件，触发重连
- 验证：`go build ./...`

## 批次 3：差异化功能（依赖批次 2）

### Task 3.1: 同步输入模式
- 文件：`internal/app/app.go`
- App 增加 `syncMode bool`、`SetSyncMode(bool)`、`IsSyncMode() bool`
- 修改 `SendCommand`：sync 模式下调用 `BroadcastCommand`
- UI：`internal/ui/window.go` 工具栏增加"同步"切换按钮
- 状态栏显示同步状态
- 验证：`go build ./...`

### Task 3.2: SFTP 双窗格
- 文件：`internal/ui/sftp_view.go`
- 新增 `localList *widget.List` 显示本地文件
- 左右 HSplit 布局：本地 | 远程
- 实现拖拽：`fyne.Draggable` + `fyne.Drop`
- 路径栏双向同步
- 右键菜单增强
- 验证：`go build ./...`

### Task 3.3: 远程进程管理器
- 新建 `internal/ui/process_view.go`
- `ProcessView struct{list *widget.Table, search *widget.Entry}`
- 通过 SSH 执行 `ps aux --sort=-%cpu | head -50`
- 表格列：PID/USER/CPU/MEM/COMMAND
- 双击行 kill 进程（确认对话框）
- 5s 自动刷新
- TerminalView 工具栏增加"进程"按钮
- 验证：`go build ./...`

### Task 3.4: 配色方案 UI
- 文件：`internal/ui/theme.go`
- Theme 增加 `colorScheme *ColorScheme`
- 设置面板（新建 `internal/ui/settings_dialog.go`）
- 下拉选择配色方案
- 应用到终端渲染
- 验证：`go build ./...`

## 批次 4：P2 功能（依赖批次 3）

### Task 4.1: 命令历史
- 新建 `internal/terminal/history.go`
- `CommandHistory struct{cmds []string, index int, file string}`
- 方法：`Add(cmd)`、`Prev() string`、`Next() string`、`Search(keyword) []string`
- 持久化到 `~/.goshell/history/`
- QuickCmdBar 集成 ↑/↓ 和 Ctrl+R
- 验证：`go build ./...`

### Task 4.2: Ping/Trace 监控
- 新建 `internal/network/ping.go`
- 使用 `golang.org/x/net/icmp` 实现 ping
- 侧边栏增加"网络"分组
- 显示延迟和丢包率
- 验证：`go build ./...`

## 批次 5：集成测试

### Task 5.1: 全局集成
- `internal/ui/window.go` 集成所有新功能
- 快捷键：Alt+1~9, Ctrl+Shift+T, Ctrl+Tab
- 验证：`go build ./... && go vet ./...`

### Task 5.2: 内存检查
- 运行时通过 `runtime.ReadMemStats` 输出内存
- 对比基准
- 验证：程序能正常启动运行
