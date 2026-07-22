package config

import (
	"time"

	"github.com/google/uuid"
)

type SessionType string

const (
	SessionSSH    SessionType = "ssh"
	SessionSerial SessionType = "serial"
	SessionTelnet SessionType = "telnet"
	SessionLocal  SessionType = "local"   // 本地终端（通过 PTY 运行系统 shell）
	SessionRLogin SessionType = "rlogin"  // RLogin 协议（RFC 1282）
	SessionFTP    SessionType = "ftp"     // FTP 文件传输协议
	SessionMosh   SessionType = "mosh"    // Mosh（Mobile Shell）
)

type AuthMethod string

const (
	AuthPassword  AuthMethod = "password"
	AuthPublicKey AuthMethod = "publickey"
	AuthKeyboard  AuthMethod = "keyboard-interactive"
)

type ProxyConfig struct {
	Type     string `json:"type"` // socks5 / http
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"` // 加密存储
}

type TunnelConfig struct {
	Type       string `json:"type"` // local / remote / dynamic
	LocalAddr  string `json:"local_addr"`
	RemoteAddr string `json:"remote_addr"`
}

type Session struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Group         string          `json:"group,omitempty"`
	Type          SessionType     `json:"type"`
	Host          string          `json:"host"`
	Port          int             `json:"port"`
	Username      string          `json:"username"`
	Password      string          `json:"password,omitempty"`       // 加密存储
	PrivateKey    string          `json:"private_key,omitempty"`    // 加密存储
	Passphrase    string          `json:"passphrase,omitempty"`     // 加密存储
	AuthMethod    AuthMethod      `json:"auth_method"`
	Shell         string          `json:"shell,omitempty"` // 本地终端使用的 shell 路径（如 /bin/bash）
	ProxyJump     string          `json:"proxy_jump,omitempty"` // 跳板机（格式: [user@]host[:port]）
	Proxy         *ProxyConfig    `json:"proxy,omitempty"`
	Tunnels       []TunnelConfig  `json:"tunnels,omitempty"`
	QuickCommands []string        `json:"quick_commands,omitempty"`
	TermType      string          `json:"term_type,omitempty"` // xterm-256color
	FontSize      float32         `json:"font_size,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

func NewSession(name string, stype SessionType) *Session {
	return &Session{
		ID:        uuid.NewString(),
		Name:      name,
		Type:      stype,
		Port:      22,
		AuthMethod: AuthPassword,
		TermType:  "xterm-256color",
		FontSize:  14,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func (s *Session) Validate() error {
	if s.Name == "" {
		return ErrValidation("session name is required")
	}
	if s.Type == "" {
		return ErrValidation("session type is required")
	}
	// 本地终端不需要主机和端口
	if s.Type == SessionLocal {
		return nil
	}
	// FTP/RLogin/Mosh/Telnet/SSH 都需要主机
	if s.Host == "" {
		return ErrValidation("host is required")
	}
	// 设置默认端口
	if s.Port == 0 {
		switch s.Type {
		case SessionSSH:
			s.Port = 22
		case SessionTelnet:
			s.Port = 23
		case SessionFTP:
			s.Port = 21
		case SessionRLogin:
			s.Port = 513
		case SessionMosh:
			s.Port = 22
		}
	}
	return nil
}

type ErrValidation string

func (e ErrValidation) Error() string { return string(e) }
