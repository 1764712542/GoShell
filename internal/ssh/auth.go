package ssh

import (
	"fmt"
	"net"
	"os"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"github.com/zhuyao/meatshell/internal/config"
	"github.com/zhuyao/meatshell/internal/log"
)

// authMethods 根据 session 配置构建 SSH 认证方法列表。
// 支持密码认证、私钥认证（OpenSSH PEM / PuTTY PPK）和 keyboard-interactive 认证。
// 多种认证方法会依次尝试，直到服务器接受。
func (w *Worker) authMethods() ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	switch w.session.AuthMethod {
	case config.AuthPassword:
		if w.session.Password == "" {
			return nil, fmt.Errorf("密码未设置")
		}
		methods = append(methods, ssh.Password(w.session.Password))
		// 同时添加 keyboard-interactive 作为备选（部分服务器仅支持此方式）
		methods = append(methods, ssh.KeyboardInteractive(w.interactiveCB))

	case config.AuthPublicKey:
		if w.session.PrivateKey == "" {
			return nil, fmt.Errorf("私钥未设置")
		}
		signer, err := parseSigner(w.session.PrivateKey, w.session.Passphrase)
		if err != nil {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
		methods = append(methods, ssh.PublicKeys(signer))

	case config.AuthKeyboard:
		methods = append(methods, ssh.KeyboardInteractive(w.interactiveCB))
		// 如果有密码，也添加密码认证作为备选
		if w.session.Password != "" {
			methods = append(methods, ssh.Password(w.session.Password))
		}

	default:
		// 默认尝试所有可用的认证方法
		if w.session.Password != "" {
			methods = append(methods, ssh.Password(w.session.Password))
		}
		if w.session.PrivateKey != "" {
			if signer, err := parseSigner(w.session.PrivateKey, w.session.Passphrase); err == nil {
				methods = append(methods, ssh.PublicKeys(signer))
			} else {
				log.Warn("parse private key failed", "err", err)
			}
		}
		methods = append(methods, ssh.KeyboardInteractive(w.interactiveCB))
	}

	// 尝试添加 ssh-agent 认证（如果系统有 SSH_AUTH_SOCK 环境变量）
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		if conn, err := net.Dial("unix", sock); err == nil {
			sshAgent := agent.NewClient(conn)
			signers, err := sshAgent.Signers()
			if err == nil && len(signers) > 0 {
				methods = append(methods, ssh.PublicKeysCallback(sshAgent.Signers))
			} else {
				conn.Close()
			}
		}
	}

	if len(methods) == 0 {
		return nil, fmt.Errorf("没有可用的认证方法")
	}

	return methods, nil
}

// interactiveCB 是 keyboard-interactive 认证的回调函数。
// 对服务器提出的所有问题返回密码作为回答。
func (w *Worker) interactiveCB(name, instruction string, questions []string, echos []bool) ([]string, error) {
	// 对所有问题返回密码
	answers := make([]string, len(questions))
	for i := range questions {
		answers[i] = w.session.Password
	}
	return answers, nil
}

// parseSigner 解析私钥，支持 OpenSSH PEM 格式和 PuTTY PPK 格式。
// 如果数据不是 PPK 格式，回退到 OpenSSH PEM 解析。
func parseSigner(privateKey, passphrase string) (ssh.Signer, error) {
	// 先尝试作为 PPK 解析
	signer, ppkErr := parsePPK([]byte(privateKey), passphrase)
	if ppkErr == nil {
		return signer, nil
	}
	if ppkErr != ErrNotPPK {
		// PPK 解析失败（可能是密码错误），记录后继续尝试 PEM
		log.Debug("ppk parse failed, trying PEM", "err", ppkErr)
	}

	// 尝试 OpenSSH PEM 格式
	if passphrase != "" {
		signer, err := ssh.ParsePrivateKeyWithPassphrase([]byte(privateKey), []byte(passphrase))
		if err != nil {
			// 如果 PPK 解析也失败了，返回组合错误
			if ppkErr != ErrNotPPK {
				return nil, fmt.Errorf("parse private key failed (ppk: %v, pem: %w)", ppkErr, err)
			}
			return nil, fmt.Errorf("parse encrypted private key: %w", err)
		}
		return signer, nil
	}

	signer, err := ssh.ParsePrivateKey([]byte(privateKey))
	if err != nil {
		if ppkErr != ErrNotPPK {
			return nil, fmt.Errorf("parse private key failed (ppk: %v, pem: %w)", ppkErr, err)
		}
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	return signer, nil
}
