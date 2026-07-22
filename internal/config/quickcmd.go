package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// QuickCmd 表示一条快捷命令
type QuickCmd struct {
	Name    string `json:"name"`
	Command string `json:"command"`
	Group   string `json:"group,omitempty"` // 分组（空表示"未分组"）
}

// quickCmdsFilePath 返回快捷命令存储文件路径
func quickCmdsFilePath() string {
	return filepath.Join(configDir(), "quickcmds.json")
}

// initQuickCmds 初始化 Store 的快捷命令字段
func initQuickCmds(s *Store) {
	s.quickCmds = make([]QuickCmd, 0)
}

// LoadQuickCmds 从磁盘加载快捷命令
func (s *Store) LoadQuickCmds() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(quickCmdsFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			s.quickCmds = make([]QuickCmd, 0)
			return nil // 首次运行，无文件
		}
		return fmt.Errorf("read quickcmds file: %w", err)
	}

	var cmds []QuickCmd
	if err := json.Unmarshal(data, &cmds); err != nil {
		return fmt.Errorf("parse quickcmds json: %w", err)
	}
	if cmds == nil {
		cmds = make([]QuickCmd, 0)
	}
	s.quickCmds = cmds
	return nil
}

// SaveQuickCmds 将快捷命令持久化到磁盘
func (s *Store) SaveQuickCmds() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := json.MarshalIndent(s.quickCmds, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal quickcmds: %w", err)
	}

	os.MkdirAll(configDir(), 0755)
	return os.WriteFile(quickCmdsFilePath(), data, 0600)
}

// ListQuickCmds 返回所有快捷命令，按 Group+Name 排序
func (s *Store) ListQuickCmds() []QuickCmd {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]QuickCmd, len(s.quickCmds))
	copy(result, s.quickCmds)
	sort.Slice(result, func(i, j int) bool {
		if result[i].Group != result[j].Group {
			return result[i].Group < result[j].Group
		}
		return result[i].Name < result[j].Name
	})
	return result
}

// AddQuickCmd 添加新快捷命令并持久化
func (s *Store) AddQuickCmd(cmd QuickCmd) error {
	if cmd.Name == "" {
		return ErrValidation("quick command name is required")
	}
	if cmd.Command == "" {
		return ErrValidation("quick command content is required")
	}

	s.mu.Lock()
	// 检查重名
	for _, existing := range s.quickCmds {
		if existing.Name == cmd.Name {
			s.mu.Unlock()
			return ErrValidation("quick command name already exists: " + cmd.Name)
		}
	}
	s.quickCmds = append(s.quickCmds, cmd)
	s.mu.Unlock()
	return s.SaveQuickCmds()
}

// UpdateQuickCmd 按 name 更新快捷命令并持久化
func (s *Store) UpdateQuickCmd(name string, cmd QuickCmd) error {
	if cmd.Name == "" {
		return ErrValidation("quick command name is required")
	}
	if cmd.Command == "" {
		return ErrValidation("quick command content is required")
	}

	s.mu.Lock()
	idx := -1
	for i, existing := range s.quickCmds {
		if existing.Name == name {
			idx = i
			break
		}
	}
	if idx == -1 {
		s.mu.Unlock()
		return ErrValidation("quick command not found: " + name)
	}
	// 如果改名了，检查新名是否冲突
	if cmd.Name != name {
		for i, existing := range s.quickCmds {
			if i != idx && existing.Name == cmd.Name {
				s.mu.Unlock()
				return ErrValidation("quick command name already exists: " + cmd.Name)
			}
		}
	}
	s.quickCmds[idx] = cmd
	s.mu.Unlock()
	return s.SaveQuickCmds()
}

// DeleteQuickCmd 按 name 删除快捷命令并持久化
func (s *Store) DeleteQuickCmd(name string) error {
	s.mu.Lock()
	idx := -1
	for i, existing := range s.quickCmds {
		if existing.Name == name {
			idx = i
			break
		}
	}
	if idx == -1 {
		s.mu.Unlock()
		return ErrValidation("quick command not found: " + name)
	}
	s.quickCmds = append(s.quickCmds[:idx], s.quickCmds[idx+1:]...)
	s.mu.Unlock()
	return s.SaveQuickCmds()
}

// ListQuickCmdGroups 返回所有分组名（去重排序）
func (s *Store) ListQuickCmdGroups() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	seen := make(map[string]struct{})
	for _, cmd := range s.quickCmds {
		seen[cmd.Group] = struct{}{}
	}
	result := make([]string, 0, len(seen))
	for g := range seen {
		result = append(result, g)
	}
	sort.Strings(result)
	return result
}
