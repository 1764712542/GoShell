package sftp

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/zhuyao/meatshell/internal/event"
)

// Client 封装 SFTP 客户端，提供文件操作能力。
// 通过 uiChan 推送传输进度事件，避免直接操作 UI。
type Client struct {
	client *sftp.Client
	uiChan chan event.UIEvent
	tabID  string
}

// NewClient 基于 SSH 客户端创建 SFTP 客户端。
func NewClient(sshClient *ssh.Client, uiChan chan event.UIEvent, tabID string) (*Client, error) {
	c, err := sftp.NewClient(sshClient)
	if err != nil {
		return nil, fmt.Errorf("create sftp client: %w", err)
	}
	return &Client{
		client: c,
		uiChan: uiChan,
		tabID:  tabID,
	}, nil
}

// Close 关闭 SFTP 客户端
func (c *Client) Close() error {
	if c.client == nil {
		return nil
	}
	return c.client.Close()
}

// List 列出远程目录下的文件，转换为 event.SFTPEntry。
func (c *Client) List(path string) ([]event.SFTPEntry, error) {
	infos, err := c.client.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", path, err)
	}

	entries := make([]event.SFTPEntry, 0, len(infos))
	for _, info := range infos {
		entries = append(entries, event.SFTPEntry{
			Name:    info.Name(),
			Size:    info.Size(),
			IsDir:   info.IsDir(),
			ModTime: info.ModTime().Unix(),
		})
	}
	return entries, nil
}

// Upload 上传本地文件到远程路径，带进度条。
func (c *Client) Upload(localPath, remotePath string) error {
	srcFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open local file %s: %w", localPath, err)
	}
	defer srcFile.Close()

	info, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("stat local file %s: %w", localPath, err)
	}

	dstFile, err := c.client.Create(remotePath)
	if err != nil {
		return fmt.Errorf("create remote file %s: %w", remotePath, err)
	}
	defer dstFile.Close()

	reader := &progressReader{
		reader:    srcFile,
		fileName:  filepath.Base(localPath),
		total:     info.Size(),
		uiChan:    c.uiChan,
		tabID:     c.tabID,
		lastSent:  time.Now(),
	}

	if _, err := io.Copy(dstFile, reader); err != nil {
		c.sendProgress(filepath.Base(localPath), info.Size(), info.Size(), true)
		return fmt.Errorf("upload copy: %w", err)
	}

	// 发送完成事件
	c.sendProgress(filepath.Base(localPath), info.Size(), info.Size(), true)
	return nil
}

// Download 下载远程文件到本地路径，带进度条。
func (c *Client) Download(remotePath, localPath string) error {
	srcFile, err := c.client.Open(remotePath)
	if err != nil {
		return fmt.Errorf("open remote file %s: %w", remotePath, err)
	}
	defer srcFile.Close()

	info, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("stat remote file %s: %w", remotePath, err)
	}

	dstFile, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("create local file %s: %w", localPath, err)
	}
	defer dstFile.Close()

	reader := &progressReader{
		reader:    srcFile,
		fileName:   filepath.Base(remotePath),
		total:      info.Size(),
		uiChan:     c.uiChan,
		tabID:      c.tabID,
		lastSent:   time.Now(),
	}

	if _, err := io.Copy(dstFile, reader); err != nil {
		c.sendProgress(filepath.Base(remotePath), info.Size(), info.Size(), true)
		return fmt.Errorf("download copy: %w", err)
	}

	// 发送完成事件
	c.sendProgress(filepath.Base(remotePath), info.Size(), info.Size(), true)
	return nil
}

// Mkdir 在远程创建目录
func (c *Client) Mkdir(path string) error {
	if err := c.client.Mkdir(path); err != nil {
		return fmt.Errorf("mkdir %s: %w", path, err)
	}
	return nil
}

// Remove 删除远程文件或目录
func (c *Client) Remove(path string) error {
	if err := c.client.Remove(path); err != nil {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

// Rename 重命名远程文件或目录
func (c *Client) Rename(oldName, newName string) error {
	if err := c.client.Rename(oldName, newName); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", oldName, newName, err)
	}
	return nil
}

// Stat 获取远程文件信息
func (c *Client) Stat(path string) (os.FileInfo, error) {
	info, err := c.client.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	return info, nil
}

// sendProgress 发送传输进度事件，限制 100ms 间隔避免事件洪流。
func (c *Client) sendProgress(fileName string, bytes, total int64, done bool) {
	select {
	case c.uiChan <- event.UIEvent{
		TabID:        c.tabID,
		Type:         event.EventSFTP,
		SFTPProgress: &event.TransferProgress{
			FileName: fileName,
			Bytes:    bytes,
			Total:    total,
			Done:     done,
		},
	}:
	default:
		// 通道已满，丢弃事件以避免阻塞
	}
}
