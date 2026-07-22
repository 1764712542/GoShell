package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// configDir 返回 meatshell 配置目录路径
func configDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "meatshell")
}

func keyFilePath() string {
	return filepath.Join(configDir(), ".machinekey")
}

func sessionsFilePath() string {
	return filepath.Join(configDir(), "sessions.json")
}

// Store 管理会话的持久化存储
type Store struct {
	mu        sync.RWMutex
	sessions  map[string]*Session
	quickCmds []QuickCmd
}

func NewStore() *Store {
	s := &Store{
		sessions: make(map[string]*Session),
	}
	initQuickCmds(s)
	return s
}

// Load 从磁盘加载会话，自动解密敏感字段
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(sessionsFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 首次运行，无文件
		}
		return fmt.Errorf("read sessions file: %w", err)
	}

	var rawSessions []*Session
	if err := json.Unmarshal(data, &rawSessions); err != nil {
		return fmt.Errorf("parse sessions json: %w", err)
	}

	s.sessions = make(map[string]*Session)
	for _, sess := range rawSessions {
		// 解密敏感字段
		if sess.Password != "" {
			if dec, err := Decrypt(sess.Password); err == nil {
				sess.Password = dec
			}
		}
		if sess.PrivateKey != "" {
			if dec, err := Decrypt(sess.PrivateKey); err == nil {
				sess.PrivateKey = dec
			}
		}
		if sess.Passphrase != "" {
			if dec, err := Decrypt(sess.Passphrase); err == nil {
				sess.Passphrase = dec
			}
		}
		if sess.Proxy != nil && sess.Proxy.Password != "" {
			if dec, err := Decrypt(sess.Proxy.Password); err == nil {
				sess.Proxy.Password = dec
			}
		}
		s.sessions[sess.ID] = sess
	}
	return nil
}

// Save 将会话持久化到磁盘，自动加密敏感字段
func (s *Store) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 复制一份并加密敏感字段
	var rawSessions []*Session
	for _, sess := range s.sessions {
		clone := *sess
		if enc, err := Encrypt(clone.Password); err == nil {
			clone.Password = enc
		}
		if enc, err := Encrypt(clone.PrivateKey); err == nil {
			clone.PrivateKey = enc
		}
		if enc, err := Encrypt(clone.Passphrase); err == nil {
			clone.Passphrase = enc
		}
		if clone.Proxy != nil && clone.Proxy.Password != "" {
			if enc, err := Encrypt(clone.Proxy.Password); err == nil {
				clone.Proxy.Password = enc
			}
		}
		rawSessions = append(rawSessions, &clone)
	}

	data, err := json.MarshalIndent(rawSessions, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal sessions: %w", err)
	}

	os.MkdirAll(configDir(), 0755)
	return os.WriteFile(sessionsFilePath(), data, 0600)
}

// List 返回所有会话，按分组和名称排序
func (s *Store) List() []*Session {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		result = append(result, sess)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Group != result[j].Group {
			return result[i].Group < result[j].Group
		}
		return result[i].Name < result[j].Name
	})
	return result
}

// Get 按 ID 获取会话
func (s *Store) Get(id string) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	return sess, ok
}

// Add 添加新会话并持久化
func (s *Store) Add(sess *Session) error {
	if err := sess.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	s.sessions[sess.ID] = sess
	s.mu.Unlock()
	return s.Save()
}

// Update 更新会话并持久化
func (s *Store) Update(sess *Session) error {
	if err := sess.Validate(); err != nil {
		return err
	}
	sess.UpdatedAt = time.Now()
	s.mu.Lock()
	s.sessions[sess.ID] = sess
	s.mu.Unlock()
	return s.Save()
}

// Delete 删除会话并持久化
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
	return s.Save()
}

// Export 导出会话为 JSON（不加密敏感字段）
func (s *Store) Export() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sessions := s.List()
	return json.MarshalIndent(struct {
		Sessions []*Session `json:"sessions"`
		Version  string     `json:"version"`
	}{
		Sessions: sessions,
		Version:  "1.0",
	}, "", "  ")
}

// Import 从 JSON 导入会话
func (s *Store) Import(data []byte, overwrite bool) error {
	var imp struct {
		Sessions []*Session `json:"sessions"`
		Version  string     `json:"version"`
	}
	if err := json.Unmarshal(data, &imp); err != nil {
		return fmt.Errorf("parse import json: %w", err)
	}

	s.mu.Lock()
	for _, sess := range imp.Sessions {
		if !overwrite {
			if _, exists := s.sessions[sess.ID]; exists {
				continue
			}
		}
		s.sessions[sess.ID] = sess
	}
	s.mu.Unlock()
	return s.Save()
}
