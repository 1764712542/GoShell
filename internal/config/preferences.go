package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// Preferences 是用户可编辑的声明式配置。
type Preferences struct {
	Font      FontConfig        `yaml:"font"`
	Theme     ThemeConfig       `yaml:"theme"`
	Terminal  TerminalConfig    `yaml:"terminal"`
	Shortcuts map[string]string `yaml:"shortcuts"`
	UI        UIConfig          `yaml:"ui"`
}

// FontConfig 描述终端字体设置
type FontConfig struct {
	Family     string  `yaml:"family"`     // 字体族，空=系统默认
	Size       float32 `yaml:"size"`       // 字号
	LineHeight float32 `yaml:"lineHeight"` // 行高倍率，默认1.0
}

// ThemeConfig 描述主题配色
type ThemeConfig struct {
	Name       string `yaml:"name"`       // 配色方案名称
	Background string `yaml:"background"` // 背景色 #RRGGBB
	Foreground string `yaml:"foreground"` // 前景色
}

// TerminalConfig 描述终端行为
type TerminalConfig struct {
	ScrollbackLines int    `yaml:"scrollbackLines"` // 回滚行数，默认10000
	TermType        string `yaml:"termType"`        // 终端类型，默认xterm-256color
	CursorStyle     string `yaml:"cursorStyle"`     // block/underline/bar
}

// UIConfig 描述界面显示选项
type UIConfig struct {
	ShowTabBar    bool   `yaml:"showTabBar"`
	ShowStatusBar bool   `yaml:"showStatusBar"`
	TabPosition   string `yaml:"tabPosition"` // top/bottom
}

// DefaultPreferences 返回默认配置
func DefaultPreferences() *Preferences {
	return &Preferences{
		Font: FontConfig{
			Size:       14,
			LineHeight: 1.0,
		},
		Theme: ThemeConfig{
			Name:       "dark",
			Background: "#1e1e2e",
			Foreground: "#d3d7cf",
		},
		Terminal: TerminalConfig{
			ScrollbackLines: 10000,
			TermType:        "xterm-256color",
			CursorStyle:     "block",
		},
		Shortcuts: map[string]string{
			"newTab":        "Ctrl+T",
			"closeTab":      "Ctrl+W",
			"nextTab":       "Ctrl+Tab",
			"prevTab":       "Ctrl+Shift+Tab",
			"copy":          "Ctrl+Shift+C",
			"paste":         "Ctrl+Shift+V",
			"search":        "Ctrl+Shift+F",
			"settings":      "Ctrl+Shift+S",
			"toggleSFTP":    "Ctrl+Shift+P",
			"fontSizeUp":    "Ctrl+=",
			"fontSizeDown":  "Ctrl+-",
			"fontSizeReset": "Ctrl+0",
		},
		UI: UIConfig{
			ShowTabBar:    true,
			ShowStatusBar: true,
			TabPosition:   "top",
		},
	}
}

// preferencesFilePath 返回配置文件路径
func preferencesFilePath() string {
	return filepath.Join(configDir(), "config.yaml")
}

// LoadPreferences 从 YAML 文件加载配置，文件不存在则返回默认值
func LoadPreferences() (*Preferences, error) {
	data, err := os.ReadFile(preferencesFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultPreferences(), nil
		}
		return nil, err
	}
	p := DefaultPreferences()
	if err := yaml.Unmarshal(data, p); err != nil {
		return nil, fmt.Errorf("parse config.yaml: %w", err)
	}
	return p, nil
}

// Save 将配置写入 YAML 文件
func (p *Preferences) Save() error {
	data, err := yaml.Marshal(p)
	if err != nil {
		return err
	}
	os.MkdirAll(configDir(), 0755)
	return os.WriteFile(preferencesFilePath(), data, 0644)
}

// PreferencesManager 管理配置的加载、热重载和订阅
type PreferencesManager struct {
	mu       sync.RWMutex
	current  *Preferences
	watchers []func(*Preferences)
}

// NewPreferencesManager 创建配置管理器并加载当前配置
func NewPreferencesManager() (*PreferencesManager, error) {
	p, err := LoadPreferences()
	if err != nil {
		return nil, err
	}
	return &PreferencesManager{current: p}, nil
}

// Get 返回当前配置快照
func (m *PreferencesManager) Get() *Preferences {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// Watch 注册配置变更回调
func (m *PreferencesManager) Watch(fn func(*Preferences)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.watchers = append(m.watchers, fn)
}

// Update 替换当前配置并通知所有订阅者
func (m *PreferencesManager) Update(p *Preferences) {
	m.mu.Lock()
	m.current = p
	watchers := make([]func(*Preferences), len(m.watchers))
	copy(watchers, m.watchers)
	m.mu.Unlock()
	for _, fn := range watchers {
		fn(p)
	}
}
