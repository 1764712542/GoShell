package ftp

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jlaffaye/ftp"

	"github.com/zhuyao/meatshell/internal/event"
)

// Client 封装 FTP 客户端，提供文件操作能力。
// 通过 uiChan 推送传输进度事件，避免直接操作 UI。
type Client struct {
	conn   *ftp.ServerConn
	uiChan chan event.UIEvent
	tabID  string
}

// NewClient 连接 FTP 服务器并登录。
func NewClient(addr string, username, password string, uiChan chan event.UIEvent, tabID string) (*Client, error) {
	conn, err := ftp.Dial(addr, ftp.DialWithTimeout(30*time.Second))
	if err != nil {
		return nil, fmt.Errorf("dial ftp %s: %w", addr, err)
	}
	if err := conn.Login(username, password); err != nil {
		conn.Quit()
		return nil, fmt.Errorf("login ftp %s: %w", addr, err)
	}
	return &Client{
		conn:   conn,
		uiChan: uiChan,
		tabID:  tabID,
	}, nil
}

// Close 关闭 FTP 连接
func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Quit()
}

// List 列出远程目录下的文件，转换为 event.SFTPEntry。
func (c *Client) List(path string) ([]event.SFTPEntry, error) {
	entries, err := c.conn.List(path)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", path, err)
	}

	result := make([]event.SFTPEntry, 0, len(entries))
	for _, e := range entries {
		// 跳过当前目录和父目录项
		if e.Name == "." || e.Name == ".." {
			continue
		}
		result = append(result, event.SFTPEntry{
			Name:    e.Name,
			Size:    int64(e.Size),
			IsDir:   e.Type == ftp.EntryTypeFolder,
			ModTime: e.Time.Unix(),
		})
	}
	return result, nil
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

	reader := &progressReader{
		reader:   srcFile,
		fileName: filepath.Base(localPath),
		total:    info.Size(),
		uiChan:   c.uiChan,
		tabID:    c.tabID,
		lastSent: time.Now(),
	}

	if err := c.conn.Stor(remotePath, reader); err != nil {
		c.sendProgress(filepath.Base(localPath), info.Size(), info.Size(), true)
		return fmt.Errorf("upload stor: %w", err)
	}

	// 发送完成事件
	c.sendProgress(filepath.Base(localPath), info.Size(), info.Size(), true)
	return nil
}

// Download 下载远程文件到本地路径，带进度条。
func (c *Client) Download(remotePath, localPath string) error {
	// 尝试获取远程文件大小用于进度显示
	var total int64
	if size, err := c.conn.FileSize(remotePath); err == nil {
		total = size
	}

	resp, err := c.conn.Retr(remotePath)
	if err != nil {
		return fmt.Errorf("retr remote file %s: %w", remotePath, err)
	}
	defer resp.Close()

	// 确保本地目录存在
	localDir := filepath.Dir(localPath)
	if err := os.MkdirAll(localDir, 0755); err != nil {
		return fmt.Errorf("mkdir local dir %s: %w", localDir, err)
	}

	dstFile, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("create local file %s: %w", localPath, err)
	}
	defer dstFile.Close()

	reader := &progressReader{
		reader:   resp,
		fileName: filepath.Base(remotePath),
		total:    total,
		uiChan:   c.uiChan,
		tabID:    c.tabID,
		lastSent: time.Now(),
	}

	n, err := io.Copy(dstFile, reader)
	if err != nil {
		c.sendProgress(filepath.Base(remotePath), n, total, true)
		return fmt.Errorf("download copy: %w", err)
	}

	// 如果总大小未知，使用实际传输字节数
	if total == 0 {
		total = n
	}
	c.sendProgress(filepath.Base(remotePath), n, total, true)
	return nil
}

// Mkdir 在远程创建目录
func (c *Client) Mkdir(path string) error {
	if err := c.conn.MakeDir(path); err != nil {
		return fmt.Errorf("mkdir %s: %w", path, err)
	}
	return nil
}

// Remove 删除远程文件或目录
func (c *Client) Remove(path string) error {
	// FTP 协议对文件和目录使用不同的命令，先尝试删除文件
	if err := c.conn.Delete(path); err != nil {
		// 失败则尝试删除目录
		if err := c.conn.RemoveDir(path); err != nil {
			return fmt.Errorf("remove %s: %w", path, err)
		}
	}
	return nil
}

// Rename 重命名远程文件或目录
func (c *Client) Rename(oldName, newName string) error {
	if err := c.conn.Rename(oldName, newName); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", oldName, newName, err)
	}
	return nil
}

// sendProgress 发送传输进度事件，限制 100ms 间隔避免事件洪流。
func (c *Client) sendProgress(fileName string, bytes, total int64, done bool) {
	select {
	case c.uiChan <- event.UIEvent{
		TabID: c.tabID,
		Type:  event.EventSFTP,
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

// progressReader 包装 io.Reader，在读取过程中发送进度事件。
// 进度事件每 100ms 最多发送一次，避免事件洪流。
type progressReader struct {
	reader   io.Reader
	fileName string
	total    int64
	read     int64
	uiChan   chan event.UIEvent
	tabID    string

	mu        sync.Mutex
	lastSent  time.Time
	startTime time.Time
}

// Read 实现 io.Reader 接口，每 100ms 发送一次进度事件。
func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.mu.Lock()
		r.read += int64(n)
		now := time.Now()
		if r.startTime.IsZero() {
			r.startTime = now
		}
		if now.Sub(r.lastSent) >= 100*time.Millisecond {
			r.lastSent = now
			elapsed := now.Sub(r.startTime).Seconds()
			var speed float64
			if elapsed > 0 {
				speed = float64(r.read) / elapsed
			}
			r.mu.Unlock()
			select {
			case r.uiChan <- event.UIEvent{
				TabID: r.tabID,
				Type:  event.EventSFTP,
				SFTPProgress: &event.TransferProgress{
					FileName: r.fileName,
					Bytes:    r.read,
					Total:    r.total,
					Speed:    speed,
					Done:     false,
				},
			}:
			default:
			}
		} else {
			r.mu.Unlock()
		}
	}
	return n, err
}
