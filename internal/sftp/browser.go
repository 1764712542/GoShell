package sftp

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zhuyao/meatshell/internal/event"
)

// Browser 维护 SFTP 文件浏览状态，包括当前目录和条目列表。
// 它通过 Client 进行远程文件系统操作，并为 UI 提供简化的浏览接口。
type Browser struct {
	client  *Client
	cwd     string           // 当前工作目录
	entries []event.SFTPEntry
}

// NewBrowser 创建一个文件浏览器，默认工作目录为根目录。
func NewBrowser(client *Client) *Browser {
	return &Browser{
		client: client,
		cwd:    "/",
	}
}

// List 列出指定目录下的文件并更新浏览器状态。
// 路径会被规范化，相对路径会基于当前工作目录解析。
func (b *Browser) List(path string) error {
	// 处理空路径或相对路径
	if path == "" {
		path = b.cwd
	}
	if !filepath.IsAbs(path) && !strings.HasPrefix(path, "/") {
		path = filepath.Join(b.cwd, path)
	}
	// 统一使用 / 作为分隔符（SFTP 路径始终使用正斜杠）
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	// 规范化路径，处理 . 和 ..
	path = filepath.Clean(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	entries, err := b.client.List(path)
	if err != nil {
		return fmt.Errorf("list %s: %w", path, err)
	}

	// 排序：目录优先，然后按名称排序
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})

	b.cwd = path
	b.entries = entries
	return nil
}

// Cwd 返回当前工作目录
func (b *Browser) Cwd() string {
	return b.cwd
}

// Entries 返回当前目录的条目列表
func (b *Browser) Entries() []event.SFTPEntry {
	return b.entries
}

// Parent 返回当前目录的父目录路径。
// 如果已经在根目录，则返回根目录。
func (b *Browser) Parent() (string, error) {
	if b.cwd == "/" {
		return "/", nil
	}
	parent := filepath.ToSlash(filepath.Dir(b.cwd))
	if !strings.HasPrefix(parent, "/") {
		parent = "/" + parent
	}
	return parent, nil
}
