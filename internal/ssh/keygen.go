package ssh

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"fmt"

	"golang.org/x/crypto/ssh"
)

// GenerateKeyPair 根据指定的密钥类型和位数生成 SSH 密钥对。
//
// 参数：
//   - keyType: "ed25519", "rsa", "ecdsa"
//   - bits: RSA 密钥位数（仅 RSA 使用，默认 4096；ECDSA 固定使用 P-256）
//
// 返回值：
//   - privateKey: OpenSSH PEM 格式的私钥
//   - publicKey:  authorized_keys 格式的公钥（单行，末尾含换行）
//   - err: 生成错误
func GenerateKeyPair(keyType string, bits int) (privateKey, publicKey string, err error) {
	switch keyType {
	case "ed25519":
		return generateEd25519Key()
	case "rsa":
		return generateRSAKey(bits)
	case "ecdsa":
		return generateECDSAKey()
	default:
		return "", "", fmt.Errorf("unsupported key type: %s (supported: ed25519, rsa, ecdsa)", keyType)
	}
}

// generateEd25519Key 生成 Ed25519 密钥对。
func generateEd25519Key() (string, string, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generate ed25519 key: %w", err)
	}

	// 序列化私钥为 OpenSSH PEM 格式
	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		return "", "", fmt.Errorf("marshal ed25519 private key: %w", err)
	}
	privatePEM := string(pem.EncodeToMemory(block))

	// 序列化公钥为 authorized_keys 格式
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return "", "", fmt.Errorf("convert ed25519 public key: %w", err)
	}
	publicAuth := string(ssh.MarshalAuthorizedKey(sshPub))

	return privatePEM, publicAuth, nil
}

// generateRSAKey 生成 RSA 密钥对。
// bits 为密钥位数，默认 4096。
func generateRSAKey(bits int) (string, string, error) {
	if bits <= 0 {
		bits = 4096
	}
	if bits < 2048 {
		return "", "", fmt.Errorf("RSA key size must be at least 2048, got %d", bits)
	}

	priv, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return "", "", fmt.Errorf("generate rsa key: %w", err)
	}

	// 序列化私钥为 OpenSSH PEM 格式
	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		return "", "", fmt.Errorf("marshal rsa private key: %w", err)
	}
	privatePEM := string(pem.EncodeToMemory(block))

	// 序列化公钥为 authorized_keys 格式
	sshPub, err := ssh.NewPublicKey(&priv.PublicKey)
	if err != nil {
		return "", "", fmt.Errorf("convert rsa public key: %w", err)
	}
	publicAuth := string(ssh.MarshalAuthorizedKey(sshPub))

	return privatePEM, publicAuth, nil
}

// generateECDSAKey 生成 ECDSA 密钥对（P-256 曲线）。
func generateECDSAKey() (string, string, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generate ecdsa key: %w", err)
	}

	// 序列化私钥为 OpenSSH PEM 格式
	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		return "", "", fmt.Errorf("marshal ecdsa private key: %w", err)
	}
	privatePEM := string(pem.EncodeToMemory(block))

	// 序列化公钥为 authorized_keys 格式
	sshPub, err := ssh.NewPublicKey(&priv.PublicKey)
	if err != nil {
		return "", "", fmt.Errorf("convert ecdsa public key: %w", err)
	}
	publicAuth := string(ssh.MarshalAuthorizedKey(sshPub))

	return privatePEM, publicAuth, nil
}

// AppendCommentToPublicKey 将注释追加到公钥末尾。
// authorized_keys 格式: ssh-ed25519 AAAA... comment
func AppendCommentToPublicKey(publicKey, comment string) string {
	if comment == "" {
		return publicKey
	}
	// 移除末尾换行，添加注释，再加回换行
	pub := publicKey
	if len(pub) > 0 && pub[len(pub)-1] == '\n' {
		pub = pub[:len(pub)-1]
	}
	return pub + " " + comment + "\n"
}
