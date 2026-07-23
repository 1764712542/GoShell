package config

import (
	"os"
	"sync"
	"testing"
	"time"
)

// resetCryptoGlobals 清空 crypto 包的全局状态，确保测试间相互隔离。
// machineKey / masterPassword / masterPasswordAttempts / lockoutUntil
// 都是包级变量，同一个测试二进制中各测试共享，必须在每个测试前后重置。
func resetCryptoGlobals() {
	keyMu.Lock()
	machineKey = nil
	masterPassword = nil
	masterPasswordAttempts = 0
	lockoutUntil = time.Time{}
	keyMu.Unlock()
}

// setupTestConfigDir 将 HOME 指向一个临时目录，避免测试污染真实配置。
// configDir() 在 macOS 上使用 $HOME/Library/Application Support，
// 因此改写 HOME 即可重定向所有配置文件落点。
// 返回的 cleanup 函数会恢复 HOME、删除临时目录并重置全局状态。
func setupTestConfigDir(t *testing.T) func() {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "goshell-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	resetCryptoGlobals()
	return func() {
		os.Setenv("HOME", oldHome)
		os.RemoveAll(tmpDir)
		resetCryptoGlobals()
	}
}

// repeatStr 返回一段较长文本，用于覆盖较长明文路径。
func repeatStr(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'x'
	}
	return string(b)
}

// TestEncryptDecrypt 验证加密后再解密能还原原文。
func TestEncryptDecrypt(t *testing.T) {
	cleanup := setupTestConfigDir(t)
	defer cleanup()

	cases := []string{
		"hello world",
		"密码 password 123",
		"a",
		repeatStr(4096),
	}
	for _, plain := range cases {
		enc, err := Encrypt(plain)
		if err != nil {
			t.Fatalf("Encrypt(%q) error: %v", plain, err)
		}
		if enc == plain && plain != "" {
			t.Fatalf("ciphertext identical to plaintext for %q", plain)
		}
		dec, err := Decrypt(enc)
		if err != nil {
			t.Fatalf("Decrypt error: %v", err)
		}
		if dec != plain {
			t.Fatalf("round-trip mismatch: got %q want %q", dec, plain)
		}
	}
}

// TestEncryptEmpty 验证空字符串加密直接返回空且无 error。
func TestEncryptEmpty(t *testing.T) {
	cleanup := setupTestConfigDir(t)
	defer cleanup()

	enc, err := Encrypt("")
	if err != nil {
		t.Fatalf("Encrypt(\"\") error: %v", err)
	}
	if enc != "" {
		t.Fatalf("Encrypt(\"\") = %q, want empty", enc)
	}

	// 空密文解密同样应返回空
	dec, err := Decrypt("")
	if err != nil {
		t.Fatalf("Decrypt(\"\") error: %v", err)
	}
	if dec != "" {
		t.Fatalf("Decrypt(\"\") = %q, want empty", dec)
	}
}

// TestDecryptInvalidBase64 验证无效 base64 输入返回 error。
func TestDecryptInvalidBase64(t *testing.T) {
	cleanup := setupTestConfigDir(t)
	defer cleanup()

	// 先触发一次 machineKey 初始化（activeKey 会写磁盘）
	if _, err := Encrypt("seed"); err != nil {
		t.Fatalf("seed Encrypt error: %v", err)
	}

	// 含非法 base64 字符，DecodeString 必定失败
	_, err := Decrypt("@@@@ not valid base64 @@@@")
	if err == nil {
		t.Fatal("Decrypt with invalid base64 should return error, got nil")
	}
}

// TestMasterPasswordLockout 验证连续 5 次验证失败后进入锁定，
// 且锁定期内即使提供正确密码也返回 false。
func TestMasterPasswordLockout(t *testing.T) {
	cleanup := setupTestConfigDir(t)
	defer cleanup()

	correctPw := "correct-master-pw"
	if err := EnableMasterPassword(correctPw); err != nil {
		t.Fatalf("EnableMasterPassword: %v", err)
	}
	// 清除内存中的主密码，确保 VerifyMasterPassword 走磁盘校验路径
	ClearMasterPassword()

	if IsMasterPasswordLockedOut() {
		t.Fatal("should not be locked out initially")
	}
	if got := MasterPasswordRemainingAttempts(); got != maxMasterPasswordAttempts {
		t.Fatalf("initial remaining attempts = %d, want %d", got, maxMasterPasswordAttempts)
	}

	// 连续失败 maxMasterPasswordAttempts 次
	for i := 1; i <= maxMasterPasswordAttempts; i++ {
		if VerifyMasterPassword("wrong-password") {
			t.Fatalf("VerifyMasterPassword with wrong password returned true on attempt %d", i)
		}
		want := maxMasterPasswordAttempts - i
		if got := MasterPasswordRemainingAttempts(); got != want {
			t.Fatalf("after %d failures, remaining = %d, want %d", i, got, want)
		}
	}

	if !IsMasterPasswordLockedOut() {
		t.Fatalf("should be locked out after %d failures", maxMasterPasswordAttempts)
	}
	if MasterPasswordLockoutRemaining() <= 0 {
		t.Fatal("lockout remaining should be positive when locked")
	}

	// 锁定期内即使输入正确密码也应失败
	if VerifyMasterPassword(correctPw) {
		t.Fatal("VerifyMasterPassword with correct password should fail during lockout")
	}
}

// TestMasterPasswordLockoutReset 验证成功验证后失败计数与锁定状态重置。
func TestMasterPasswordLockoutReset(t *testing.T) {
	cleanup := setupTestConfigDir(t)
	defer cleanup()

	correctPw := "reset-correct-pw"
	if err := EnableMasterPassword(correctPw); err != nil {
		t.Fatalf("EnableMasterPassword: %v", err)
	}
	ClearMasterPassword()

	// 制造 3 次失败（未达上限）
	for i := 0; i < 3; i++ {
		VerifyMasterPassword("bad")
	}
	if got := MasterPasswordRemainingAttempts(); got != maxMasterPasswordAttempts-3 {
		t.Fatalf("remaining before reset = %d, want %d", got, maxMasterPasswordAttempts-3)
	}

	// 用正确密码验证，应成功并重置计数
	if !VerifyMasterPassword(correctPw) {
		t.Fatal("VerifyMasterPassword with correct password should succeed")
	}
	if IsMasterPasswordLockedOut() {
		t.Fatal("should not be locked out after successful verify")
	}
	if got := MasterPasswordRemainingAttempts(); got != maxMasterPasswordAttempts {
		t.Fatalf("remaining after reset = %d, want %d", got, maxMasterPasswordAttempts)
	}
}

// TestConcurrentEncryptDecrypt 在 -race 下验证并发加密/解密无 data race。
func TestConcurrentEncryptDecrypt(t *testing.T) {
	cleanup := setupTestConfigDir(t)
	defer cleanup()

	// 预热：先初始化 machineKey，避免并发首次初始化时过多竞争
	if _, err := Encrypt("warmup"); err != nil {
		t.Fatalf("warmup Encrypt: %v", err)
	}

	const goroutines = 16
	const iterations = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				plain := "concurrent-data"
				enc, err := Encrypt(plain)
				if err != nil {
					t.Errorf("goroutine %d Encrypt: %v", id, err)
					return
				}
				dec, err := Decrypt(enc)
				if err != nil {
					t.Errorf("goroutine %d Decrypt: %v", id, err)
					return
				}
				if dec != plain {
					t.Errorf("goroutine %d round-trip mismatch: got %q want %q", id, dec, plain)
					return
				}
			}
		}(g)
	}
	wg.Wait()
}

// TestSetMasterPassword 验证设置主密码后 IsMasterPasswordSet 返回 true。
func TestSetMasterPassword(t *testing.T) {
	cleanup := setupTestConfigDir(t)
	defer cleanup()

	if IsMasterPasswordSet() {
		t.Fatal("master password should not be set initially")
	}
	SetMasterPassword("my-secret")
	if !IsMasterPasswordSet() {
		t.Fatal("IsMasterPasswordSet should return true after SetMasterPassword")
	}

	// 设置主密码后，加密/解密应使用主密码派生密钥
	enc, err := Encrypt("protected")
	if err != nil {
		t.Fatalf("Encrypt after SetMasterPassword: %v", err)
	}
	dec, err := Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt after SetMasterPassword: %v", err)
	}
	if dec != "protected" {
		t.Fatalf("round-trip after SetMasterPassword: got %q want %q", dec, "protected")
	}
}

// TestClearMasterPassword 验证清除后 IsMasterPasswordSet 返回 false。
func TestClearMasterPassword(t *testing.T) {
	cleanup := setupTestConfigDir(t)
	defer cleanup()

	SetMasterPassword("temp-pw")
	if !IsMasterPasswordSet() {
		t.Fatal("expected master password set before clear")
	}
	ClearMasterPassword()
	if IsMasterPasswordSet() {
		t.Fatal("IsMasterPasswordSet should return false after ClearMasterPassword")
	}
}

// TestMachineKeyInit 验证机器密钥初始化的幂等性：
// 多次调用返回 nil error，且持久化的密钥在重新加载后保持一致。
func TestMachineKeyInit(t *testing.T) {
	cleanup := setupTestConfigDir(t)
	defer cleanup()

	// 首次初始化：生成随机密钥并写入磁盘
	if err := initMachineKey(); err != nil {
		t.Fatalf("first initMachineKey: %v", err)
	}
	keyMu.RLock()
	firstKey := make([]byte, len(machineKey))
	copy(firstKey, machineKey)
	keyMu.RUnlock()
	if len(firstKey) != 32 {
		t.Fatalf("machineKey length = %d, want 32", len(firstKey))
	}

	// 幂等：再次调用应直接返回（machineKey 已在内存）
	if err := initMachineKey(); err != nil {
		t.Fatalf("second initMachineKey: %v", err)
	}
	keyMu.RLock()
	secondKey := make([]byte, len(machineKey))
	copy(secondKey, machineKey)
	keyMu.RUnlock()
	if string(secondKey) != string(firstKey) {
		t.Fatal("machineKey changed on idempotent second init")
	}

	// 清除内存后重新初始化，应从磁盘读回同一密钥
	resetCryptoGlobals()
	if err := initMachineKey(); err != nil {
		t.Fatalf("initMachineKey after reset: %v", err)
	}
	keyMu.RLock()
	loadedKey := make([]byte, len(machineKey))
	copy(loadedKey, machineKey)
	keyMu.RUnlock()
	if string(loadedKey) != string(firstKey) {
		t.Fatal("machineKey loaded from disk differs from originally generated key")
	}
}


// TestFingerprint 验证主机密钥指纹的确定性与区分度。
func TestFingerprint(t *testing.T) {
	cleanup := setupTestConfigDir(t)
	defer cleanup()

	key := []byte("test-host-key-data")
	fp := Fingerprint(key)
	if fp == "" {
		t.Fatal("Fingerprint returned empty string")
	}
	// 确定性：相同输入应产生相同输出
	if Fingerprint(key) != fp {
		t.Fatal("Fingerprint not deterministic for same input")
	}
	// 区分度：不同输入应产生不同输出
	if Fingerprint([]byte("different-key")) == fp {
		t.Fatal("Fingerprint collision for different inputs")
	}
}

// TestIsMasterPasswordEnabled 验证磁盘令牌存在性判断。
func TestIsMasterPasswordEnabled(t *testing.T) {
	cleanup := setupTestConfigDir(t)
	defer cleanup()

	if IsMasterPasswordEnabled() {
		t.Fatal("should not be enabled before EnableMasterPassword")
	}
	if err := EnableMasterPassword("pw"); err != nil {
		t.Fatalf("EnableMasterPassword: %v", err)
	}
	if !IsMasterPasswordEnabled() {
		t.Fatal("should be enabled after EnableMasterPassword")
	}
}

// TestChangeMasterPassword 验证更换主密码后旧密码失效、新密码生效，
// 且会话数据经重新加密后仍可正常解密。
func TestChangeMasterPassword(t *testing.T) {
	cleanup := setupTestConfigDir(t)
	defer cleanup()

	s := NewStore()
	sess := NewSession("change-pw", SessionSSH)
	sess.Host = "h"
	sess.Password = "session-secret"
	if err := s.Add(sess); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if err := EnableMasterPassword("old-master"); err != nil {
		t.Fatalf("EnableMasterPassword: %v", err)
	}

	if err := s.ChangeMasterPassword("old-master", "new-master"); err != nil {
		t.Fatalf("ChangeMasterPassword: %v", err)
	}

	// 新密码应能通过校验
	if !VerifyMasterPassword("new-master") {
		t.Fatal("VerifyMasterPassword with new password should succeed")
	}
	// 旧密码应失败
	if VerifyMasterPassword("old-master") {
		t.Fatal("VerifyMasterPassword with old password should fail")
	}

	// 会话数据重新加载后密码仍可正确解密
	s2 := NewStore()
	if err := s2.Load(); err != nil {
		t.Fatalf("Load after change: %v", err)
	}
	got, ok := s2.Get(sess.ID)
	if !ok {
		t.Fatal("session not found after master password change")
	}
	if got.Password != "session-secret" {
		t.Fatalf("Password after change = %q, want %q", got.Password, "session-secret")
	}
}

// TestDisableMasterPassword 验证禁用后令牌文件删除、内存密钥清除，
// 且会话回退到机器密钥加密仍可正常加载。
func TestDisableMasterPassword(t *testing.T) {
	cleanup := setupTestConfigDir(t)
	defer cleanup()

	s := NewStore()
	sess := NewSession("disable-pw", SessionSSH)
	sess.Host = "h"
	sess.Password = "plain-secret"
	if err := s.Add(sess); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := EnableMasterPassword("to-disable"); err != nil {
		t.Fatalf("EnableMasterPassword: %v", err)
	}
	if !IsMasterPasswordEnabled() {
		t.Fatal("should be enabled before disable")
	}

	if err := s.DisableMasterPassword(); err != nil {
		t.Fatalf("DisableMasterPassword: %v", err)
	}

	if IsMasterPasswordEnabled() {
		t.Fatal("should not be enabled after disable")
	}
	if IsMasterPasswordSet() {
		t.Fatal("should not be set in memory after disable")
	}

	// 会话用机器密钥重新加密，应能正常加载
	s2 := NewStore()
	if err := s2.Load(); err != nil {
		t.Fatalf("Load after disable: %v", err)
	}
	got, ok := s2.Get(sess.ID)
	if !ok {
		t.Fatal("session not found after disable")
	}
	if got.Password != "plain-secret" {
		t.Fatalf("Password after disable = %q, want %q", got.Password, "plain-secret")
	}
}
