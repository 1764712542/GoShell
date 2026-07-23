package config

import (
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// WatchConfigFile 监听 config.yaml 文件变化，变化时重新加载并通知订阅者。
// 返回一个 stop 函数用于停止监听。stop 可被多次调用，仅首次生效。
func (m *PreferencesManager) WatchConfigFile() (func() error, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	// 监听配置目录（监听目录而非文件本身，可正确捕获编辑器原子替换文件的事件）
	if err := watcher.Add(configDir()); err != nil {
		watcher.Close()
		return nil, err
	}

	done := make(chan struct{})
	var once sync.Once

	go func() {
		for {
			select {
			case <-done:
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				// 只关心 config.yaml 的写/创建/重命名事件
				if filepath.Base(event.Name) != "config.yaml" {
					continue
				}
				if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
					continue
				}
				// 重新加载。失败则保留旧配置，避免坏文件覆盖运行时状态
				if p, err := LoadPreferences(); err == nil {
					m.Update(p)
				}
			case _, ok := <-watcher.Errors:
				if !ok {
					return
				}
				// 出错时仅退出 goroutine，stop 仍可正常清理
				return
			}
		}
	}()

	stop := func() error {
		once.Do(func() {
			close(done)
			watcher.Close()
		})
		return nil
	}
	return stop, nil
}
