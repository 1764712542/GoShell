package ssh

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/zhuyao/meatshell/internal/event"
	"github.com/zhuyao/meatshell/internal/config"
	"github.com/zhuyao/meatshell/internal/log"
)

// hostKeyCallback 是 SSH 连接的主机密钥校验回调。
// 流程：
//  1. 如果 known_hosts 文件中已有该主机的记录，比对密钥是否一致
//  2. 如果密钥不匹配，拒绝连接（可能存在中间人攻击）
//  3. 如果是首次连接（无记录），发送 EventStatus 通知 UI 弹窗确认
//  4. 用户确认后写入 known_hosts 并允许连接
func (w *Worker) hostKeyCallback(hostname string, remote net.Addr, key ssh.PublicKey) error {
	knownHostsPath := knownHostsFilePath()

	// 如果 known_hosts 文件存在，检查主机密钥
	if _, err := os.Stat(knownHostsPath); err == nil {
		callback, err := knownhosts.New(knownHostsPath)
		if err != nil {
			return fmt.Errorf("create known_hosts callback: %w", err)
		}

		err = callback(hostname, remote, key)
		if err == nil {
			// 主机密钥已知且匹配
			return nil
		}

		if keyErr, ok := err.(*knownhosts.KeyError); ok && len(keyErr.Want) > 0 {
			// 主机密钥不匹配，可能存在中间人攻击
			return fmt.Errorf("主机 %s 的密钥已变更，可能存在安全风险", hostname)
		}
		// 如果是 KeyError 且 Want 为空，说明主机未知，继续到下面的提示流程
	}

	// 首次连接，提示用户确认
	return w.promptHostKey(hostname, key)
}

// promptHostKey 发送主机密钥确认请求到 UI，阻塞等待用户响应。
// 使用 select + ctx.Done() 避免在关闭过程中阻塞。
func (w *Worker) promptHostKey(hostname string, key ssh.PublicKey) error {
	keyInfo := &event.HostKeyInfo{
		Host:        hostname,
		Fingerprint: config.Fingerprint(key.Marshal()),
		KeyType:     key.Type(),
	}

	// 发送主机密钥确认事件到 UI（非阻塞，避免关闭时死锁）
	select {
	case w.uiChan <- event.UIEvent{
		TabID:   w.session.ID,
		Type:    event.EventStatus,
		Status:  event.StatusHostKeyPrompt,
		HostKey: keyInfo,
	}:
	case <-w.ctx.Done():
		return fmt.Errorf("主机密钥确认已取消")
	default:
		// 通道已满，无法发送
		return fmt.Errorf("UI 事件通道已满，无法发送主机密钥确认请求")
	}

	log.Info("waiting for host key confirmation", "host", hostname, "fingerprint", keyInfo.Fingerprint)

	// 等待用户确认
	select {
	case accepted := <-w.hostKeyConfirm:
		if !accepted {
			return fmt.Errorf("用户拒绝了主机密钥")
		}
	case <-w.ctx.Done():
		return fmt.Errorf("主机密钥确认已取消")
	}

	// 用户确认，写入 known_hosts
	if err := w.addToKnownHosts(hostname, key); err != nil {
		log.Warn("写入 known_hosts 失败", "err", err)
		// 即使写入失败也继续连接
	}

	return nil
}

// addToKnownHosts 将主机密钥追加到 known_hosts 文件
func (w *Worker) addToKnownHosts(hostname string, key ssh.PublicKey) error {
	knownHostsPath := knownHostsFilePath()

	// 确保 ~/.ssh 目录存在
	if err := os.MkdirAll(filepath.Dir(knownHostsPath), 0700); err != nil {
		return fmt.Errorf("create known_hosts directory: %w", err)
	}

	// 使用 knownhosts.Line 格式化条目
	line := knownhosts.Line([]string{hostname}, key)

	// 追加到文件
	f, err := os.OpenFile(knownHostsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open known_hosts: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(line + "\n"); err != nil {
		return fmt.Errorf("write known_hosts: %w", err)
	}

	log.Info("host key added to known_hosts", "host", hostname)
	return nil
}

// ConfirmHostKey 响应主机密钥确认请求，由 UI 调用
func (w *Worker) ConfirmHostKey(accepted bool) {
	select {
	case w.hostKeyConfirm <- accepted:
	case <-w.ctx.Done():
	}
}

// knownHostsFilePath 返回 known_hosts 文件路径（~/.ssh/known_hosts）
func knownHostsFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "known_hosts"
	}
	return filepath.Join(home, ".ssh", "known_hosts")
}
