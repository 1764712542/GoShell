package ssh

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/zhuyao/meatshell/internal/log"
)

// buildSSHConfig 构建 SSH 客户端配置（认证方法、主机密钥校验、加密套件）。
// 供 dial() 和 dialViaJumpHost() 共用。
func (w *Worker) buildSSHConfig() (*ssh.ClientConfig, error) {
	sshConfig := &ssh.ClientConfig{
		User:            w.session.Username,
		HostKeyCallback: w.hostKeyCallback,
		Timeout:         30 * time.Second,
		Config: ssh.Config{
			Ciphers: []string{
				"aes128-ctr", "aes192-ctr", "aes256-ctr",
				"aes128-gcm@openssh.com",
				"chacha20-poly1305@openssh.com",
			},
			KeyExchanges: []string{
				"curve25519-sha256", "curve25519-sha256@libssh.org",
				"ecdh-sha2-nistp256", "ecdh-sha2-nistp384", "ecdh-sha2-nistp521",
				"diffie-hellman-group-exchange-sha256",
			},
		},
	}

	// 获取认证方法
	methods, err := w.authMethods()
	if err != nil {
		return nil, fmt.Errorf("build auth methods: %w", err)
	}
	sshConfig.Auth = methods

	return sshConfig, nil
}

// dial 建立 SSH 连接并返回 ssh.Client。
// 流程：
//  1. 构建 SSH 客户端配置（认证方法、主机密钥校验）
//  2. 如果配置了 ProxyJump，通过跳板机建立连接；否则通过代理（如有）或直连
//  3. 调用 ssh.NewClientConn 完成 SSH 握手
func (w *Worker) dial(ctx context.Context) (*ssh.Client, error) {
	// 如果配置了跳板机，通过跳板机建立连接
	if w.session.ProxyJump != "" {
		return w.dialViaJumpHost(ctx)
	}

	// 构建 SSH 客户端配置
	sshConfig, err := w.buildSSHConfig()
	if err != nil {
		return nil, err
	}

	// 构建目标地址
	target := fmt.Sprintf("%s:%d", w.session.Host, w.session.Port)

	log.Info("connecting to ssh server", "host", w.session.Host, "port", w.session.Port)

	// 通过代理或直连建立 TCP 连接
	conn, err := dialViaProxy(w.session.Proxy, target)
	if err != nil {
		return nil, fmt.Errorf("dial tcp: %w", err)
	}

	// 检查 context 是否已取消
	select {
	case <-ctx.Done():
		conn.Close()
		return nil, ctx.Err()
	default:
	}

	// 建立 SSH 连接
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, target, sshConfig)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("ssh connect: %w", err)
	}

	client := ssh.NewClient(sshConn, chans, reqs)
	log.Info("ssh connected", "host", w.session.Host, "port", w.session.Port)

	return client, nil
}
