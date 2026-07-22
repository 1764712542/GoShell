package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// SSHConfigEntry 表示 ~/.ssh/config 中的一组 Host 配置
type SSHConfigEntry struct {
	Host         string
	HostName     string
	User         string
	Port         int
	IdentityFile string
	ProxyJump    string
}

// ImportSSHConfig 解析 ~/.ssh/config 文件，返回会话列表
func ImportSSHConfig() ([]*Session, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home dir: %w", err)
	}

	path := filepath.Join(home, ".ssh", "config")
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // 无配置文件，返回空
		}
		return nil, fmt.Errorf("open ssh config: %w", err)
	}
	defer file.Close()

	var entries []*SSHConfigEntry
	var current *SSHConfigEntry

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		key := strings.ToLower(parts[0])
		value := strings.Join(parts[1:], " ")

		switch key {
		case "host":
			if current != nil {
				entries = append(entries, current)
			}
			current = &SSHConfigEntry{Host: value, Port: 22}
		case "hostname":
			if current != nil {
				current.HostName = value
			}
		case "user":
			if current != nil {
				current.User = value
			}
		case "port":
			if current != nil {
				if p, err := strconv.Atoi(value); err == nil {
					current.Port = p
				}
			}
		case "identityfile":
			if current != nil {
				// 展开 ~ 路径
				if strings.HasPrefix(value, "~/") {
					value = filepath.Join(home, value[2:])
				}
				current.IdentityFile = value
			}
		case "proxyjump":
			if current != nil {
				current.ProxyJump = value
			}
		}
	}
	if current != nil {
		entries = append(entries, current)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan ssh config: %w", err)
	}

	// 转换为 Session
	var sessions []*Session
	for _, e := range entries {
		// 跳过通配符主机
		if strings.Contains(e.Host, "*") || strings.Contains(e.Host, "?") {
			continue
		}
		// 跳过没有 HostName 的（Host 本身就是主机名的情况除外）
		hostName := e.HostName
		if hostName == "" {
			hostName = e.Host
		}

		sess := NewSession(e.Host, SessionSSH)
		sess.Host = hostName
		if e.User != "" {
			sess.Username = e.User
		}
		sess.Port = e.Port
		sess.ProxyJump = e.ProxyJump
		sess.AuthMethod = AuthPublicKey

		// 读取私钥文件内容
		if e.IdentityFile != "" {
			if keyData, err := os.ReadFile(e.IdentityFile); err == nil {
				sess.PrivateKey = string(keyData)
			}
		}

		sessions = append(sessions, sess)
	}

	return sessions, nil
}
