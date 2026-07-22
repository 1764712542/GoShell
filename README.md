# GoShell

> 基于 Go + Fyne v2 的生产级跨平台终端客户端，支持 SSH / 串口 / Telnet / RLogin / FTP / Mosh / 本地终端等 7 种协议。

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Fyne](https://img.shields.io/badge/Fyne-v2.8.0-1E90FF)](https://fyne.io/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Windows%20%7C%20Linux-lightgrey)]()
[![Release](https://img.shields.io/badge/release-v1.0.0-blue)]()

GoShell 是一款轻量级跨平台终端客户端，原生 Go 编译，**无 JVM 开销**，比 FinalShell / XShell 更省内存。所有会话凭据采用 AES-256-GCM 加密 + Argon2id 密钥派生保护，开箱即用，安全可靠。

---

## 目录

- [功能特性](#功能特性)
- [截图](#截图)
- [安装与构建](#安装与构建)
- [使用方法](#使用方法)
- [配置文件位置](#配置文件位置)
- [技术架构](#技术架构)
- [项目结构](#项目结构)
- [开发](#开发)
- [许可证](#许可证)
- [致谢](#致谢)

---

## 功能特性

### 协议支持

| 协议 | 说明 |
|------|------|
| **SSH** | SSH2 完整支持，密码 / 公钥 / Keyboard-interactive 认证，代理跳板 |
| **Serial** | 串口终端，可配置波特率 / 数据位 / 停止位 / 流控 |
| **Telnet** | 传统 Telnet 协议 |
| **RLogin** | RFC 1282 远程登录协议 |
| **FTP** | 文件传输协议，支持主动 / 被动模式 |
| **Mosh** | 移动 Shell，断线自动重连 |
| **本地终端** | 本机 Shell（bash / zsh / PowerShell / cmd） |

### 核心功能

| 模块 | 特性 |
|------|------|
| **会话管理** | 分组管理 / 关键字搜索 / 导入导出 / 复制会话 / 拖拽排序 |
| **标签页** | 右键菜单 / 颜色标记 / 拖拽排序 / 复制 / 重命名 |
| **终端模拟** | ANSI 256 色 + Truecolor / 回滚缓冲区 / 复制粘贴 / 内容搜索 / 分屏 |
| **SFTP** | 双窗格文件管理 / 递归目录传输 / 拖拽上传 / 远程编辑 / chmod 权限修改 |
| **SSH 隧道** | 本地转发（-L）/ 远程转发（-R）/ 动态 SOCKS5 转发（-D） |
| **安全加密** | AES-256-GCM 加密存储 / 主密码保护 / Argon2id 密钥派生 |
| **系统监控** | CPU / MEM / NET / DISK 实时图表 / Ping 延迟检测 / 进程列表 |
| **UI 主题** | 11 种配色方案 / 暗色亮色主题切换 / 毛玻璃效果 |
| **快捷键** | 完整的键盘快捷键体系，高效操作 |
| **同步输入** | 广播命令到所有终端，批量执行 |
| **终端日志** | 会话录制 / 重放回放 |
| **宏录制** | 录制和回放键盘操作，自动化重复任务 |
| **SSH 密钥生成** | Ed25519 / RSA / ECDSA 密钥对生成 |
| **跨平台** | 原生支持 macOS / Windows / Linux |

---

## 截图

![截图](docs/screenshot.png)

---

## 安装与构建

### 方式一：下载预编译包

前往 [Releases 页面](../../releases) 下载对应平台的预编译包：

- **macOS**：`goshell-{version}-macos-{arch}.zip`
- **Linux**：`goshell-{version}-linux-{arch}.tar.gz`
- **Windows**：`goshell-{version}-windows-x86_64.zip`

解压后即可运行，无需安装额外运行时。

### 方式二：从源码构建

**依赖要求：**

- [Go](https://go.dev/dl/) 1.21+
- CGO 工具链（macOS 自带 Xcode CLT；Linux 需 `gcc`；Windows 需 MinGW-w64）

**构建命令：**

```bash
# 克隆仓库
git clone https://github.com/zhuyao/meatshell.git
cd meatshell

# 安装依赖
go mod download

# 编译
go build -o goshell ./cmd/meatshell/

# 运行
./goshell
```

或使用 Makefile：

```bash
make build    # 编译当前平台
make run      # 编译并运行
```

---

## 使用方法

### 首次启动

1. 双击运行 `goshell`（或 `goshell.exe`）
2. 首次启动会引导设置**主密码**，用于加密所有会话凭据
3. 主密码设置完成后进入主界面

### 创建会话

1. 点击左侧边栏的 **「+」** 按钮，或右键分组选择「新建会话」
2. 选择协议类型（SSH / Serial / Telnet / RLogin / FTP / Mosh / 本地终端）
3. 填写连接信息：
   - **SSH**：主机地址、端口、用户名、认证方式（密码 / 公钥）
   - **Serial**：串口设备、波特率、数据位、停止位、流控
   - **本地终端**：选择 Shell 类型
4. 点击「保存」后双击会话即可连接

### 快捷键

| 快捷键 | 功能 |
|--------|------|
| `Ctrl + T` | 新建标签页 |
| `Ctrl + W` | 关闭当前标签页 |
| `Ctrl + Tab` | 切换到下一个标签页 |
| `Ctrl + Shift + Tab` | 切换到上一个标签页 |
| `Ctrl + N` | 新建会话 |
| `Ctrl + F` | 终端内容搜索 |
| `Ctrl + Shift + C` | 复制选中内容 |
| `Ctrl + Shift + V` | 粘贴 |
| `Ctrl + Shift + S` | SFTP 文件浏览器 |
| `Ctrl + Shift + T` | SSH 隧道面板 |
| `Ctrl + Shift + M` | 系统监控面板 |
| `Ctrl + Shift + R` | 终端日志录制 |
| `Ctrl + Shift + H` | 命令历史 |
| `Ctrl + =` | 放大字体 |
| `Ctrl + -` | 缩小字体 |
| `Ctrl + 0` | 重置字体大小 |
| `Alt + Enter` | 全屏切换 |
| `F11` | 全屏切换 |

---

## 配置文件位置

GoShell 的配置文件（会话、密钥、偏好设置）按平台存储于以下目录：

| 平台 | 路径 |
|------|------|
| **macOS** | `~/Library/Application Support/meatshell/` |
| **Linux** | `~/.config/meatshell/` |
| **Windows** | `%APPDATA%\meatshell\` |

目录结构：

```
meatshell/
├── sessions.json      # 加密的会话配置
├── master.key         # Argon2id 派生的主密钥（AES-256-GCM 加密）
├── known_hosts        # SSH known_hosts
├── terminal.log       # 终端录制日志
└── preferences.json   # 用户偏好设置
```

> **注意**：主密码遗失后无法恢复会话凭据，请妥善保管。

---

## 技术架构

| 层级 | 技术选型 | 说明 |
|------|----------|------|
| **GUI 框架** | Fyne v2.8.0 | 跨平台原生渲染，单一代码库支持三平台 |
| **终端模拟** | vt10x | VT100/VT220 终端模拟器，支持 ANSI 全色彩 |
| **SSH 协议** | golang.org/x/crypto/ssh | 官方 SSH 实现，支持全部认证方式 |
| **SFTP** | pkg/sftp | SFTP 客户端，递归目录传输 |
| **串口** | go.bug.st/serial | 跨平台串口通信 |
| **FTP** | jlaffaye/ftp | FTP 客户端 |
| **加密** | AES-256-GCM + Argon2id | 军用级加密与抗暴力破解密钥派生 |
| **系统监控** | gopsutil | 跨平台系统指标采集 |
| **本地终端** | creack/pty | 伪终端，支持本地 Shell |
| **国际化** | go-i18n | 中英文双语支持 |

### 架构概览

```
┌─────────────────────────────────────────────┐
│                  UI 层 (Fyne)                │
│  ┌─────────┬──────────┬────────┬──────────┐ │
│  │ Sidebar │  Tabs    │Terminal│   SFTP   │ │
│  │ 会话管理 │ 标签管理  │  视图  │  浏览器  │ │
│  └─────────┴──────────┴────────┴──────────┘ │
├─────────────────────────────────────────────┤
│                App 控制层                    │
│     (会话调度 / 事件分发 / 标签生命周期)       │
├─────────────────────────────────────────────┤
│                协议 Worker 层                │
│  ┌─────┬──────┬───────┬──────┬─────┬──────┐ │
│  │ SSH │Telnet│Serial │RLogin│ FTP │ Mosh │ │
│  └─────┴──────┴───────┴──────┴─────┴──────┘ │
├─────────────────────────────────────────────┤
│                基础服务层                    │
│  Config(加密) │ Monitor │ I18n │ Log │ UI  │
└─────────────────────────────────────────────┘
```

---

## 项目结构

```
GoShell/
├── cmd/
│   └── meatshell/
│       └── main.go                # 程序入口
├── internal/
│   ├── app/
│   │   ├── app.go                 # 应用控制器
│   │   ├── event.go               # 事件系统
│   │   └── tab.go                 # 标签页管理
│   ├── config/
│   │   ├── crypto.go              # AES-256-GCM 加密
│   │   ├── quickcmd.go            # 快捷命令
│   │   ├── session.go             # 会话模型
│   │   ├── sshconfig.go           # SSH 配置解析
│   │   └── store.go               # 配置存储
│   ├── event/
│   │   └── event.go               # 事件类型定义
│   ├── ftp/
│   │   ├── client.go              # FTP 客户端
│   │   └── worker.go              # FTP Worker
│   ├── i18n/
│   │   ├── i18n.go                # 国际化管理
│   │   └── strings.go             # 字符串常量
│   ├── localterminal/
│   │   └── worker.go              # 本地终端 Worker
│   ├── log/
│   │   └── log.go                 # 日志系统
│   ├── monitor/
│   │   ├── local.go               # 本地监控
│   │   ├── ping.go                # Ping 延迟检测
│   │   ├── process.go             # 进程列表
│   │   ├── remote.go              # 远程监控
│   │   └── types.go               # 监控类型定义
│   ├── mosh/
│   │   └── worker.go              # Mosh Worker
│   ├── rlogin/
│   │   └── worker.go              # RLogin Worker
│   ├── serial/
│   │   └── serial.go              # 串口通信
│   ├── sftp/
│   │   ├── browser.go             # SFTP 浏览器
│   │   ├── client.go              # SFTP 客户端
│   │   └── transfer.go            # 文件传输
│   ├── ssh/
│   │   ├── auth.go                # SSH 认证
│   │   ├── client.go              # SSH 客户端
│   │   ├── knownhosts.go          # known_hosts 管理
│   │   ├── ppk.go                 # PuTTY PPK 格式
│   │   ├── proxy.go               # 代理跳板
│   │   ├── tunnel.go              # SSH 隧道
│   │   └── worker.go              # SSH Worker
│   ├── telnet/
│   │   └── telnet.go              # Telnet 协议
│   ├── terminal/
│   │   ├── emulator.go            # 终端模拟器
│   │   ├── input.go               # 输入处理
│   │   ├── render.go              # 渲染器
│   │   ├── widget.go              # 终端组件
│   │   └── zmodem.go              # ZMODEM 传输
│   ├── terminallog/
│   │   └── logger.go              # 终端日志录制
│   ├── ui/
│   │   ├── cmd_history.go         # 命令历史
│   │   ├── colorscheme_dialog.go  # 配色方案对话框
│   │   ├── colorschemes.go        # 11 种配色方案
│   │   ├── process_view.go        # 进程视图
│   │   ├── quickcmd.go            # 快捷命令
│   │   ├── session_dialog.go      # 会话编辑对话框
│   │   ├── sftp_view.go           # SFTP 视图
│   │   ├── sidebar.go             # 侧边栏
│   │   ├── sparkline.go           # 迷你图表
│   │   ├── splitter.go            # 分割器
│   │   ├── tabs.go                # 标签页
│   │   ├── terminal_view.go       # 终端视图
│   │   ├── theme.go               # 主题
│   │   ├── tunnel_panel.go        # 隧道面板
│   │   ├── welcome.go             # 欢迎页
│   │   └── window.go              # 主窗口
│   └── update/
│       └── checker.go             # 更新检查
├── lang/
│   ├── en-US.json                 # 英文语言包
│   └── zh-CN.json                 # 中文语言包
├── packaging/
│   ├── darwin/
│   │   └── Info.plist             # macOS 应用配置
│   ├── linux/
│   │   ├── goshell.desktop        # Linux 桌面入口
│   │   └── install.sh             # 安装脚本
│   └── windows/
│       └── build.ps1              # Windows 构建脚本
├── .goreleaser.yml                # GoReleaser 发布配置
├── Makefile                       # 构建脚本
├── go.mod                         # Go 模块定义
└── go.sum                         # 依赖校验
```

---

## 开发

### 开发环境搭建

```bash
# 1. 安装 Go 1.21+
brew install go          # macOS
# 或从 https://go.dev/dl/ 下载

# 2. 安装 CGO 依赖
xcode-select --install   # macOS
sudo apt install gcc libc6-dev  # Linux (Debian/Ubuntu)

# 3. 克隆仓库
git clone https://github.com/zhuyao/meatshell.git
cd meatshell

# 4. 安装依赖
go mod download

# 5. 运行（调试模式）
go run ./cmd/meatshell/ -debug -lang zh-CN
```

### 构建命令

```bash
make build              # 编译当前平台
make run                # 编译并运行
make test               # 运行测试
make vet                # 代码静态检查
make tidy               # 整理依赖
make clean              # 清理构建产物
make package-mac        # 打包 macOS .app
make package-linux      # 打包 Linux
make package-windows    # 交叉编译 Windows
make package-all        # 打包所有平台
```

### 交叉编译

由于 Fyne 依赖 CGO，交叉编译需要对应平台的 C 工具链：

```bash
# Windows（需 mingw-w64）
CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc CXX=x86_64-w64-mingw32-g++ \
GOOS=windows GOARCH=amd64 go build -o goshell.exe ./cmd/meatshell/

# Linux amd64（在 macOS 上需安装 Linux 交叉编译工具链）
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o goshell-linux ./cmd/meatshell/

# 使用 GoReleaser 一键发布
make release
```

### 命令行参数

```bash
./goshell -debug        # 启用调试日志
./goshell -lang en-US   # 指定界面语言（zh-CN / en-US）
```

---

## 许可证

本项目基于 [MIT License](LICENSE) 开源。

---

## 致谢

GoShell 的诞生离不开以下优秀的开源项目：

- [Fyne](https://fyne.io/) — 跨平台 GUI 框架
- [golang.org/x/crypto/ssh](https://pkg.go.dev/golang.org/x/crypto/ssh) — SSH 协议实现
- [vt10x](https://github.com/hinshun/vt10x) — VT 终端模拟器
- [pkg/sftp](https://github.com/pkg/sftp) — SFTP 实现
- [gopsutil](https://github.com/shirou/gopsutil) — 系统监控
- [go.bug.st/serial](https://github.com/bugst/go-serial) — 串口通信
- [creack/pty](https://github.com/creack/pty) — 伪终端

同时感谢原 **meatshell** Rust 项目提供的灵感与设计参考。本项目以 Go 重写，在保留核心功能的基础上优化了内存占用与跨平台体验。

---

<p align="center">如果 GoShell 对你有帮助，欢迎 ⭐ Star 支持一下！</p>
