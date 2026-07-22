package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/crypto/argon2"
)

var (
	// machineKey 在首次运行时生成并持久化，用于加密会话密码
	machineKey []byte
	// masterPassword 在用户启用主密码后由 Argon2id 派生，优先于 machineKey 使用
	masterPassword []byte
)

// masterPasswordVerificationToken 是用于校验主密码是否正确的已知明文。
const masterPasswordVerificationToken = "MEATSHELL_MASTER_PW_VERIFICATION"

func initMachineKey() error {
	if machineKey != nil {
		return nil
	}

	keyPath := keyFilePath()
	data, err := os.ReadFile(keyPath)
	if err == nil && len(data) == 32 {
		machineKey = data
		return nil
	}

	// 首次运行：生成随机密钥
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return err
	}
	machineKey = key

	os.MkdirAll(configDir(), 0755)
	return os.WriteFile(keyPath, key, 0600)
}

// Encrypt 使用 AES-256-GCM 加密明文。
// 若主密码已设置则使用主密码派生密钥，否则回退到机器密钥。
func Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	key, err := activeKey()
	if err != nil {
		return "", err
	}
	return encryptWithKey(plaintext, key)
}

// Decrypt 解密 AES-256-GCM 密文。
// 若主密码已设置则使用主密码派生密钥，否则回退到机器密钥。
func Decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}

	key, err := activeKey()
	if err != nil {
		return "", err
	}
	return decryptWithKey(ciphertext, key)
}

// activeKey 返回当前应使用的加密密钥：主密码优先，否则使用机器密钥。
func activeKey() ([]byte, error) {
	if masterPassword != nil {
		return masterPassword, nil
	}
	if err := initMachineKey(); err != nil {
		return nil, err
	}
	return machineKey, nil
}

// encryptWithKey 使用指定密钥加密明文（AES-256-GCM）
func encryptWithKey(plaintext string, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decryptWithKey 使用指定密钥解密密文（AES-256-GCM）
func decryptWithKey(ciphertext string, key []byte) (string, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// Fingerprint 计算主机密钥指纹（SHA-256）
func Fingerprint(key []byte) string {
	h := sha256.Sum256(key)
	return base64.StdEncoding.EncodeToString(h[:])
}

// machineID 返回机器标识，用作主密码派生密钥的盐源。
// 使用主机名作为机器标识，若获取失败则回退到固定字符串。
func machineID() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "meatshell-default"
	}
	return hostname
}

// masterPasswordFilePath 返回主密码校验令牌文件的路径。
func masterPasswordFilePath() string {
	return filepath.Join(configDir(), ".masterpw")
}

// deriveMasterKey 使用 Argon2id 从密码和机器特定盐派生 32 字节密钥。
// salt = sha256(machineID)
func deriveMasterKey(password string) []byte {
	mid := machineID()
	salt := sha256.Sum256([]byte(mid))
	return argon2.IDKey([]byte(password), salt[:], 1, 64*1024, 4, 32)
}

// SetMasterPassword 根据用户输入的密码派生 Argon2id 密钥并设置为当前主密码。
// 注意：此函数仅设置内存中的密钥，不会持久化校验令牌。
// 如需启用主密码（首次设置），请使用 EnableMasterPassword。
func SetMasterPassword(password string) {
	masterPassword = deriveMasterKey(password)
}

// EnableMasterPassword 设置主密码并持久化校验令牌到磁盘。
// 此后 Encrypt/Decrypt 将使用主密码派生密钥而非机器密钥。
func EnableMasterPassword(password string) error {
	SetMasterPassword(password)
	return saveMasterPasswordToken()
}

// saveMasterPasswordToken 将用当前主密码加密的校验令牌写入磁盘。
func saveMasterPasswordToken() error {
	if masterPassword == nil {
		return errors.New("master password not set")
	}
	token, err := encryptWithKey(masterPasswordVerificationToken, masterPassword)
	if err != nil {
		return err
	}
	os.MkdirAll(configDir(), 0755)
	return os.WriteFile(masterPasswordFilePath(), []byte(token), 0600)
}

// IsMasterPasswordSet 返回内存中是否已设置主密码密钥。
func IsMasterPasswordSet() bool {
	return masterPassword != nil
}

// IsMasterPasswordEnabled 返回磁盘上是否存在主密码校验令牌，
// 即用户是否曾启用过主密码保护。
func IsMasterPasswordEnabled() bool {
	_, err := os.Stat(masterPasswordFilePath())
	return err == nil
}

// VerifyMasterPassword 校验给定密码是否能正确解密已存储的校验令牌。
// 若主密码从未启用（令牌文件不存在），返回 false。
func VerifyMasterPassword(pw string) bool {
	data, err := os.ReadFile(masterPasswordFilePath())
	if err != nil {
		return false
	}
	key := deriveMasterKey(pw)
	plaintext, err := decryptWithKey(string(data), key)
	if err != nil {
		return false
	}
	return plaintext == masterPasswordVerificationToken
}

// ChangeMasterPassword 校验旧密码后切换到新密码，并重新加密所有会话。
// 若主密码当前已设置，必须提供正确的旧密码；首次设置时 oldPw 可为空。
// 此方法需要 Store 以便重新加密并持久化所有会话。
func (s *Store) ChangeMasterPassword(oldPw, newPw string) error {
	// 若主密码已启用，校验旧密码
	if IsMasterPasswordEnabled() {
		if !VerifyMasterPassword(oldPw) {
			return errors.New("incorrect master password")
		}
	}

	// 设置新主密码
	SetMasterPassword(newPw)

	// 持久化新的校验令牌
	if err := saveMasterPasswordToken(); err != nil {
		return err
	}

	// 重新加密所有会话并保存（Store 中的会话在内存中为明文，Save 时会使用新密钥加密）
	return s.Save()
}

// ClearMasterPassword 清除内存中的主密码密钥（不影响磁盘上的校验令牌）。
// 通常在锁定或退出时调用。
func ClearMasterPassword() {
	masterPassword = nil
}

// DisableMasterPassword 禁用主密码保护：清除内存密钥、删除校验令牌文件，
// 并使用机器密钥重新加密所有会话。
func (s *Store) DisableMasterPassword() error {
	// 先清除主密码，使 Save 使用机器密钥
	ClearMasterPassword()

	// 删除校验令牌文件
	if err := os.Remove(masterPasswordFilePath()); err != nil && !os.IsNotExist(err) {
		return err
	}

	// 使用机器密钥重新加密所有会话
	return s.Save()
}
