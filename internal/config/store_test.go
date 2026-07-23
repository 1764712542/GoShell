package config

import (
	"os"
	"sync"
	"testing"
)

// TestStoreAddGet 验证添加会话后能按 ID 获取到，且字段保持一致。
func TestStoreAddGet(t *testing.T) {
	cleanup := setupTestConfigDir(t)
	defer cleanup()

	s := NewStore()
	sess := NewSession("my-ssh", SessionSSH)
	sess.Host = "example.com"
	sess.Port = 2222
	sess.Username = "alice"
	sess.Password = "s3cret"

	if err := s.Add(sess); err != nil {
		t.Fatalf("Add: %v", err)
	}

	got, ok := s.Get(sess.ID)
	if !ok {
		t.Fatalf("Get(%s) not found after Add", sess.ID)
	}
	if got.Name != "my-ssh" {
		t.Fatalf("Name = %q, want %q", got.Name, "my-ssh")
	}
	if got.Host != "example.com" {
		t.Fatalf("Host = %q, want %q", got.Host, "example.com")
	}
	if got.Password != "s3cret" {
		t.Fatalf("Password = %q, want %q", got.Password, "s3cret")
	}
}

// TestStoreSaveLoad 验证保存后重新加载会话数据一致（含敏感字段解密还原）。
func TestStoreSaveLoad(t *testing.T) {
	cleanup := setupTestConfigDir(t)
	defer cleanup()

	s1 := NewStore()
	a := NewSession("alpha", SessionSSH)
	a.Host = "host-a"
	a.Port = 22
	a.Username = "user-a"
	a.Password = "pw-a"
	a.PrivateKey = "-----BEGIN PRIVATE KEY-----\nfake\n"
	if err := s1.Add(a); err != nil {
		t.Fatalf("Add alpha: %v", err)
	}

	b := NewSession("beta", SessionTelnet)
	b.Host = "host-b"
	b.Port = 23
	b.Username = "user-b"
	b.Password = "pw-b"
	if err := s1.Add(b); err != nil {
		t.Fatalf("Add beta: %v", err)
	}

	// 新建 Store 并从磁盘加载
	s2 := NewStore()
	if err := s2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	gotA, ok := s2.Get(a.ID)
	if !ok {
		t.Fatal("alpha not found after Load")
	}
	if gotA.Password != "pw-a" {
		t.Fatalf("alpha Password after Load = %q, want %q (decryption issue?)", gotA.Password, "pw-a")
	}
	if gotA.PrivateKey != "-----BEGIN PRIVATE KEY-----\nfake\n" {
		t.Fatalf("alpha PrivateKey after Load mismatch")
	}

	gotB, ok := s2.Get(b.ID)
	if !ok {
		t.Fatal("beta not found after Load")
	}
	if gotB.Password != "pw-b" {
		t.Fatalf("beta Password after Load = %q, want %q", gotB.Password, "pw-b")
	}
}

// TestStoreDelete 验证删除后无法再获取。
func TestStoreDelete(t *testing.T) {
	cleanup := setupTestConfigDir(t)
	defer cleanup()

	s := NewStore()
	sess := NewSession("to-delete", SessionSSH)
	sess.Host = "h"
	if err := s.Add(sess); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if err := s.Delete(sess.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, ok := s.Get(sess.ID); ok {
		t.Fatal("session still found after Delete")
	}
}

// TestStoreList 验证列表按分组、名称排序。
func TestStoreList(t *testing.T) {
	cleanup := setupTestConfigDir(t)
	defer cleanup()

	s := NewStore()
	// 故意乱序添加
	items := []struct {
		group, name string
	}{
		{"B", "b2"},
		{"A", "a2"},
		{"B", "b1"},
		{"A", "a1"},
		{"", "z-no-group"},
	}
	for _, it := range items {
		sess := NewSession(it.name, SessionSSH)
		sess.Group = it.group
		sess.Host = "h"
		if err := s.Add(sess); err != nil {
			t.Fatalf("Add %s: %v", it.name, err)
		}
	}

	list := s.List()
	if len(list) != len(items) {
		t.Fatalf("List length = %d, want %d", len(list), len(items))
	}

	// 期望顺序：空组在最前，然后 A/a1, A/a2, B/b1, B/b2, z-no-group
	wantNames := []string{"z-no-group", "a1", "a2", "b1", "b2"}
	for i, w := range wantNames {
		if list[i].Name != w {
			t.Fatalf("List[%d].Name = %q, want %q (full order: %v)", i, list[i].Name, w, namesOf(list))
		}
	}
}

func namesOf(list []*Session) []string {
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = s.Name
	}
	return out
}

// TestStoreSaveEncryptionFailure 验证加密失败时 Save 返回 error，
// 且不会把明文密码写入磁盘。
func TestStoreSaveEncryptionFailure(t *testing.T) {
	cleanup := setupTestConfigDir(t)
	defer cleanup()

	s := NewStore()
	sess := NewSession("enc-fail", SessionSSH)
	sess.Host = "h"
	sess.Password = "plaintext-secret-123"

	// 直接写入 map，绕过 Add 内部的 Save，以便单独测试 Save 的加密失败路径
	s.mu.Lock()
	s.sessions[sess.ID] = sess
	s.mu.Unlock()

	// 破坏加密：设置一个非法长度的主密码，使 aes.NewCipher 在 encryptWithKey 中失败。
	// masterPassword 非 nil 时 activeKey() 会返回它，1 字节密钥无法构造 AES。
	keyMu.Lock()
	masterPassword = []byte{0x01}
	keyMu.Unlock()

	err := s.Save()
	if err == nil {
		t.Fatal("Save should return error when encryption fails, got nil")
	}

	// 验证磁盘上不存在明文密码
	if data, rerr := os.ReadFile(sessionsFilePath()); rerr == nil {
		if contains(string(data), "plaintext-secret-123") {
			t.Fatal("plaintext password leaked to disk despite encryption failure")
		}
	}
	// 即使文件不存在也算通过（加密失败时不应写文件）
}

// contains 是一个简单的子串检查，避免在测试中引入 strings 包。
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return len(sub) == 0
}

// TestStoreConcurrentAccess 在 -race 下验证并发读写无 data race。
func TestStoreConcurrentAccess(t *testing.T) {
	cleanup := setupTestConfigDir(t)
	defer cleanup()

	s := NewStore()
	// 预置一个会话
	seed := NewSession("seed", SessionSSH)
	seed.Host = "h"
	s.mu.Lock()
	s.sessions[seed.ID] = seed
	s.mu.Unlock()

	const goroutines = 8
	const iterations = 200
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// 写者：通过锁直接操作 map（模拟并发 Add 的内存效果，避免磁盘 I/O 拖慢）
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				sess := NewSession("w", SessionSSH)
				sess.Host = "h"
				s.mu.Lock()
				s.sessions[sess.ID] = sess
				s.mu.Unlock()
			}
		}(g)
	}

	// 读者：并发 Get / List
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				s.Get(seed.ID)
				_ = s.List()
			}
		}(g)
	}

	wg.Wait()

	// 最终状态应可正常列出
	if list := s.List(); len(list) < 1 {
		t.Fatalf("List after concurrent access = %d, want >= 1", len(list))
	}
}


// TestStoreUpdate 验证更新会话后字段变更且持久化。
func TestStoreUpdate(t *testing.T) {
	cleanup := setupTestConfigDir(t)
	defer cleanup()

	s := NewStore()
	sess := NewSession("upd", SessionSSH)
	sess.Host = "old-host"
	sess.Username = "u"
	if err := s.Add(sess); err != nil {
		t.Fatalf("Add: %v", err)
	}

	sess.Host = "new-host"
	sess.Username = "updated-user"
	if err := s.Update(sess); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, ok := s.Get(sess.ID)
	if !ok {
		t.Fatal("session not found after Update")
	}
	if got.Host != "new-host" {
		t.Fatalf("Host = %q, want %q", got.Host, "new-host")
	}
	if got.Username != "updated-user" {
		t.Fatalf("Username = %q, want %q", got.Username, "updated-user")
	}
	// UpdatedAt 应被刷新
	if !got.UpdatedAt.After(got.CreatedAt) && !got.UpdatedAt.Equal(got.CreatedAt) {
		t.Fatal("UpdatedAt should be >= CreatedAt after Update")
	}

	// 重新加载验证持久化
	s2 := NewStore()
	if err := s2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	got2, _ := s2.Get(sess.ID)
	if got2.Host != "new-host" {
		t.Fatalf("Host after reload = %q, want %q", got2.Host, "new-host")
	}
}

// TestStoreExportImport 验证导出后导入会话数据一致。
func TestStoreExportImport(t *testing.T) {
	cleanup := setupTestConfigDir(t)
	defer cleanup()

	s1 := NewStore()
	sess := NewSession("exp", SessionSSH)
	sess.Host = "export-host"
	sess.Username = "eu"
	sess.Password = "ep"
	if err := s1.Add(sess); err != nil {
		t.Fatalf("Add: %v", err)
	}

	data, err := s1.Export()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("Export returned empty data")
	}

	// 导入到新 Store
	s2 := NewStore()
	if err := s2.Import(data, true); err != nil {
		t.Fatalf("Import: %v", err)
	}
	got, ok := s2.Get(sess.ID)
	if !ok {
		t.Fatal("imported session not found")
	}
	if got.Name != "exp" {
		t.Fatalf("Name = %q, want %q", got.Name, "exp")
	}
	if got.Host != "export-host" {
		t.Fatalf("Host = %q, want %q", got.Host, "export-host")
	}

	// 重新加载验证导入数据已持久化
	s3 := NewStore()
	if err := s3.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	got3, ok := s3.Get(sess.ID)
	if !ok {
		t.Fatal("imported session not found after reload")
	}
	if got3.Password != "ep" {
		t.Fatalf("Password after reload = %q, want %q", got3.Password, "ep")
	}
}

// TestStoreImportNoOverwrite 验证 overwrite=false 时已存在的会话不被覆盖。
func TestStoreImportNoOverwrite(t *testing.T) {
	cleanup := setupTestConfigDir(t)
	defer cleanup()

	s1 := NewStore()
	sess := NewSession("keep", SessionSSH)
	sess.Host = "original"
	s1.Add(sess)
	data, err := s1.Export()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	// 新 Store 中已有同名（同 ID）会话但内容不同
	s2 := NewStore()
	sess2 := *sess // 复制，同 ID
	sess2.Host = "modified"
	s2.mu.Lock()
	s2.sessions[sess2.ID] = &sess2
	s2.mu.Unlock()

	// overwrite=false，已存在的 ID 应被跳过
	if err := s2.Import(data, false); err != nil {
		t.Fatalf("Import: %v", err)
	}
	got, _ := s2.Get(sess.ID)
	if got.Host != "modified" {
		t.Fatalf("Host = %q, want %q (should not be overwritten)", got.Host, "modified")
	}
}


// TestSessionValidate 覆盖 Session.Validate 的各分支：
// 名称为空、类型为空、本地终端免主机、主机为空、默认端口推断。
func TestSessionValidate(t *testing.T) {
	cleanup := setupTestConfigDir(t)
	defer cleanup()

	// 名称为空应报错
	emptyName := NewSession("", SessionSSH)
	emptyName.Host = "h"
	if err := emptyName.Validate(); err == nil {
		t.Fatal("Validate with empty name should return error")
	}

	// 类型为空应报错
	emptyType := NewSession("s", "")
	emptyType.Host = "h"
	if err := emptyType.Validate(); err == nil {
		t.Fatal("Validate with empty type should return error")
	}

	// 本地终端不需要主机
	local := NewSession("local", SessionLocal)
	if err := local.Validate(); err != nil {
		t.Fatalf("Validate local session should succeed: %v", err)
	}

	// 非 local 但主机为空应报错
	noHost := NewSession("nohost", SessionSSH)
	if err := noHost.Validate(); err == nil {
		t.Fatal("Validate with empty host for SSH should return error")
	}

	// 默认端口推断：各协议
	cases := []struct {
		stype    SessionType
		wantPort int
	}{
		{SessionSSH, 22},
		{SessionTelnet, 23},
		{SessionFTP, 21},
		{SessionRLogin, 513},
		{SessionMosh, 22},
	}
	for _, c := range cases {
		s := NewSession("p", c.stype)
		s.Host = "h"
		s.Port = 0
		if err := s.Validate(); err != nil {
			t.Fatalf("Validate %s: %v", c.stype, err)
		}
		if s.Port != c.wantPort {
			t.Fatalf("%s default port = %d, want %d", c.stype, s.Port, c.wantPort)
		}
	}
}

// TestNewSession 验证 NewSession 设置合理的默认值。
func TestNewSession(t *testing.T) {
	s := NewSession("defaults", SessionSSH)
	if s.ID == "" {
		t.Fatal("ID should be non-empty")
	}
	if s.Name != "defaults" {
		t.Fatalf("Name = %q, want %q", s.Name, "defaults")
	}
	if s.Type != SessionSSH {
		t.Fatalf("Type = %q, want %q", s.Type, SessionSSH)
	}
	if s.Port != 22 {
		t.Fatalf("Port = %d, want 22", s.Port)
	}
	if s.AuthMethod != AuthPassword {
		t.Fatalf("AuthMethod = %q, want %q", s.AuthMethod, AuthPassword)
	}
	if s.TermType != "xterm-256color" {
		t.Fatalf("TermType = %q, want xterm-256color", s.TermType)
	}
	if s.FontSize != 14 {
		t.Fatalf("FontSize = %v, want 14", s.FontSize)
	}
	if s.CreatedAt.IsZero() || s.UpdatedAt.IsZero() {
		t.Fatal("CreatedAt/UpdatedAt should be set")
	}
}

// TestErrValidation 验证验证错误类型实现 error 接口并保留消息。
func TestErrValidation(t *testing.T) {
	err := ErrValidation("bad input")
	if err.Error() != "bad input" {
		t.Fatalf("Error() = %q, want %q", err.Error(), "bad input")
	}
}


// TestQuickCmds 覆盖快捷命令的增删改查与持久化往返。
func TestQuickCmds(t *testing.T) {
	cleanup := setupTestConfigDir(t)
	defer cleanup()

	s := NewStore()

	// 空名/空命令应报错
	if err := s.AddQuickCmd(QuickCmd{Name: "", Command: "c"}); err == nil {
		t.Fatal("AddQuickCmd with empty name should error")
	}
	if err := s.AddQuickCmd(QuickCmd{Name: "n", Command: ""}); err == nil {
		t.Fatal("AddQuickCmd with empty command should error")
	}

	// 添加
	if err := s.AddQuickCmd(QuickCmd{Name: "ls", Command: "ls -la", Group: "g1"}); err != nil {
		t.Fatalf("AddQuickCmd ls: %v", err)
	}
	if err := s.AddQuickCmd(QuickCmd{Name: "pwd", Command: "pwd", Group: "g2"}); err != nil {
		t.Fatalf("AddQuickCmd pwd: %v", err)
	}

	// 重名应报错
	if err := s.AddQuickCmd(QuickCmd{Name: "ls", Command: "x"}); err == nil {
		t.Fatal("AddQuickCmd duplicate name should error")
	}

	// 列表按 Group+Name 排序
	list := s.ListQuickCmds()
	if len(list) != 2 {
		t.Fatalf("ListQuickCmds = %d, want 2", len(list))
	}
	if list[0].Name != "ls" || list[1].Name != "pwd" {
		t.Fatalf("order = %s, %s; want ls, pwd", list[0].Name, list[1].Name)
	}

	// 分组列表
	groups := s.ListQuickCmdGroups()
	if len(groups) != 2 {
		t.Fatalf("ListQuickCmdGroups = %d, want 2", len(groups))
	}

	// 更新
	if err := s.UpdateQuickCmd("ls", QuickCmd{Name: "ls", Command: "ls -lh", Group: "g1"}); err != nil {
		t.Fatalf("UpdateQuickCmd: %v", err)
	}
	list = s.ListQuickCmds()
	if list[0].Command != "ls -lh" {
		t.Fatalf("after update Command = %q, want %q", list[0].Command, "ls -lh")
	}

	// 持久化往返
	s2 := NewStore()
	if err := s2.LoadQuickCmds(); err != nil {
		t.Fatalf("LoadQuickCmds: %v", err)
	}
	list2 := s2.ListQuickCmds()
	if len(list2) != 2 {
		t.Fatalf("after reload ListQuickCmds = %d, want 2", len(list2))
	}

	// 删除
	if err := s.DeleteQuickCmd("pwd"); err != nil {
		t.Fatalf("DeleteQuickCmd: %v", err)
	}
	if len(s.ListQuickCmds()) != 1 {
		t.Fatalf("after delete ListQuickCmds = %d, want 1", len(s.ListQuickCmds()))
	}
}

// TestLoadQuickCmdsMissing 验证文件不存在时加载返回空列表而非错误。
func TestLoadQuickCmdsMissing(t *testing.T) {
	cleanup := setupTestConfigDir(t)
	defer cleanup()

	s := NewStore()
	if err := s.LoadQuickCmds(); err != nil {
		t.Fatalf("LoadQuickCmds with no file: %v", err)
	}
	if got := s.ListQuickCmds(); len(got) != 0 {
		t.Fatalf("expected empty list, got %d", len(got))
	}
}
