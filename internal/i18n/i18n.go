package i18n

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Manager 管理国际化翻译字符串。
// 从 lang/ 目录加载 JSON 翻译文件，支持按 key 查找翻译。
type Manager struct {
	strings map[string]string
	lang    string
}

// NewManager 创建一个新的 i18n 管理器，默认语言为中文。
func NewManager() *Manager {
	return &Manager{
		strings: make(map[string]string),
		lang:    "zh-CN",
	}
}

// Load 加载指定语言的翻译文件。
// 查找顺序：
//  1. 可执行文件同级目录的 lang/ 文件夹
//  2. 配置目录（UserConfigDir/meatshell）下的 lang/ 文件夹
func (m *Manager) Load(lang string) error {
	fileName := fmt.Sprintf("%s.json", lang)

	// 候选路径列表
	candidates := []string{
		filepath.Join(exeDir(), "lang", fileName),
		filepath.Join(configDir(), "lang", fileName),
	}

	var data []byte
	var err error
	for _, path := range candidates {
		data, err = os.ReadFile(path)
		if err == nil {
			break
		}
	}
	if data == nil {
		return fmt.Errorf("load i18n file %s: not found in any candidate path", fileName)
	}

	var strings map[string]string
	if err := json.Unmarshal(data, &strings); err != nil {
		return fmt.Errorf("parse i18n json %s: %w", fileName, err)
	}

	m.strings = strings
	m.lang = lang
	return nil
}

// T 返回指定 key 的翻译。如果 key 不存在，返回 key 本身。
func (m *Manager) T(key string) string {
	if v, ok := m.strings[key]; ok {
		return v
	}
	return key
}

// Tf 返回格式化后的翻译。
// 使用 fmt.Sprintf 对翻译字符串进行格式化。
func (m *Manager) Tf(key string, args ...interface{}) string {
	tmpl := m.T(key)
	return fmt.Sprintf(tmpl, args...)
}

// AvailableLangs 返回可用语言列表。
// 扫描可执行文件同级目录和配置目录的 lang/ 文件夹。
func (m *Manager) AvailableLangs() []string {
	seen := make(map[string]bool)
	var langs []string

	// 扫描候选目录
	dirs := []string{
		filepath.Join(exeDir(), "lang"),
		filepath.Join(configDir(), "lang"),
	}

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if !strings.HasSuffix(name, ".json") {
				continue
			}
			lang := name[:len(name)-5] // 去掉 .json 后缀
			if !seen[lang] {
				seen[lang] = true
				langs = append(langs, lang)
			}
		}
	}

	// 保证至少返回默认语言
	if len(langs) == 0 {
		langs = []string{"zh-CN", "en-US"}
	}

	sort.Strings(langs)
	return langs
}

// CurrentLang 返回当前使用的语言代码
func (m *Manager) CurrentLang() string {
	return m.lang
}

// exeDir 返回可执行文件所在目录
func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

// configDir 返回 meatshell 配置目录路径
func configDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "meatshell")
}
