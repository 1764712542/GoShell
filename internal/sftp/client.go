package sftp

import (
	"fmt"
	"io"
	"os"
	"path"
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

// UploadDir 递归上传本地目录到远程路径。
// 遍历 localDir 下的所有文件和子目录，在远程创建对应的目录结构并上传文件。
func (c *Client) UploadDir(localDir, remoteDir string) error {
	info, err := os.Stat(localDir)
	if err != nil {
		return fmt.Errorf("stat local dir %s: %w", localDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("upload dir: %s is not a directory", localDir)
	}

	// 确保远程根目录存在
	if err := c.client.MkdirAll(remoteDir); err != nil {
		return fmt.Errorf("mkdir remote %s: %w", remoteDir, err)
	}

	return filepath.Walk(localDir, func(localPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 计算相对路径并拼接远程路径
		relPath, err := filepath.Rel(localDir, localPath)
		if err != nil {
			return err
		}
		remotePath := path.Join(remoteDir, filepath.ToSlash(relPath))

		if info.IsDir() {
			if err := c.client.MkdirAll(remotePath); err != nil {
				return fmt.Errorf("mkdir remote %s: %w", remotePath, err)
			}
			return nil
		}

		return c.Upload(localPath, remotePath)
	})
}

// DownloadDir 递归下载远程目录到本地路径。
// 遍历 remoteDir 下的所有文件和子目录，在本地创建对应的目录结构并下载文件。
func (c *Client) DownloadDir(remoteDir, localDir string) error {
	// 创建本地根目录
	if err := os.MkdirAll(localDir, 0755); err != nil {
		return fmt.Errorf("mkdir local %s: %w", localDir, err)
	}

	infos, err := c.client.ReadDir(remoteDir)
	if err != nil {
		return fmt.Errorf("read dir %s: %w", remoteDir, err)
	}

	for _, info := range infos {
		remotePath := path.Join(remoteDir, info.Name())
		localPath := filepath.Join(localDir, info.Name())

		if info.IsDir() {
			if err := c.DownloadDir(remotePath, localPath); err != nil {
				return err
			}
		} else {
			if err := c.Download(remotePath, localPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// WalkRemote 递归遍历远程目录，返回所有文件（不含目录）的完整路径。
func (c *Client) WalkRemote(remotePath string) ([]string, error) {
	infos, err := c.client.ReadDir(remotePath)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", remotePath, err)
	}

	var files []string
	for _, info := range infos {
		fullPath := path.Join(remotePath, info.Name())
		if info.IsDir() {
			subFiles, err := c.WalkRemote(fullPath)
			if err != nil {
				return nil, err
			}
			files = append(files, subFiles...)
		} else {
			files = append(files, fullPath)
		}
	}
	return files, nil
}

// Chmod 修改远程文件或目录的权限
func (c *Client) Chmod(remotePath string, mode os.FileMode) error {
	if err := c.client.Chmod(remotePath, mode); err != nil {
		return fmt.Errorf("chmod %s: %w", remotePath, err)
	}
	return nil
}

// ReadFile 读取远程文件内容到内存
func (c *Client) ReadFile(remotePath string) ([]byte, error) {
	f, err := c.client.Open(remotePath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", remotePath, err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", remotePath, err)
	}
	return data, nil
}

// WriteFile 将数据写入远程文件（覆盖已有内容）
func (c *Client) WriteFile(remotePath string, data []byte) error {
	f, err := c.client.Create(remotePath)
	if err != nil {
		return fmt.Errorf("create %s: %w", remotePath, err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write %s: %w", remotePath, err)
	}
	return nil
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
