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
)

var (
	// machineKey 在首次运行时生成并持久化，用于加密会话密码
	machineKey []byte
)

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

// Encrypt 使用 AES-256-GCM 加密明文（ChaCha20-Poly1305 也可，这里用 AES-GCM 因 Go 标准库更方便）
func Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if err := initMachineKey(); err != nil {
		return "", err
	}

	block, err := aes.NewCipher(machineKey)
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

// Decrypt 解密 AES-256-GCM 密文
func Decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	if err := initMachineKey(); err != nil {
		return "", err
	}

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(machineKey)
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
