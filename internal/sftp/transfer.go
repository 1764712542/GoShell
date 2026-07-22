package sftp

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/zhuyao/meatshell/internal/event"
)

// Transfer 管理文件传输，封装 Client 提供带进度的上传/下载。
type Transfer struct {
	client *Client
	uiChan chan event.UIEvent
	tabID  string
}

// NewTransfer 创建传输管理器
func NewTransfer(client *Client, uiChan chan event.UIEvent, tabID string) *Transfer {
	return &Transfer{
		client: client,
		uiChan: uiChan,
		tabID:  tabID,
	}
}

// UploadFile 上传本地文件到远程路径，使用自定义 Reader 跟踪进度。
func (t *Transfer) UploadFile(localPath, remotePath string) error {
	srcFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open local file %s: %w", localPath, err)
	}
	defer srcFile.Close()

	info, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("stat local file %s: %w", localPath, err)
	}

	// 确保远程目录存在
	remoteDir := filepath.ToSlash(filepath.Dir(remotePath))
	if remoteDir != "." && remoteDir != "/" {
		t.client.Mkdir(remoteDir)
	}

	dstFile, err := t.client.client.Create(remotePath)
	if err != nil {
		return fmt.Errorf("create remote file %s: %w", remotePath, err)
	}
	defer dstFile.Close()

	reader := &progressReader{
		reader:    srcFile,
		fileName:  filepath.Base(localPath),
		total:     info.Size(),
		uiChan:    t.uiChan,
		tabID:     t.tabID,
		lastSent:  time.Now(),
	}

	if _, err := io.Copy(dstFile, reader); err != nil {
		t.sendProgress(filepath.Base(localPath), info.Size(), info.Size(), true)
		return fmt.Errorf("upload copy: %w", err)
	}

	// 发送完成事件
	t.sendProgress(filepath.Base(localPath), info.Size(), info.Size(), true)
	return nil
}

// DownloadFile 下载远程文件到本地路径，使用自定义 Reader 跟踪进度。
func (t *Transfer) DownloadFile(remotePath, localPath string) error {
	srcFile, err := t.client.client.Open(remotePath)
	if err != nil {
		return fmt.Errorf("open remote file %s: %w", remotePath, err)
	}
	defer srcFile.Close()

	info, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("stat remote file %s: %w", remotePath, err)
	}

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
		reader:    srcFile,
		fileName:   filepath.Base(remotePath),
		total:      info.Size(),
		uiChan:     t.uiChan,
		tabID:      t.tabID,
		lastSent:   time.Now(),
	}

	if _, err := io.Copy(dstFile, reader); err != nil {
		t.sendProgress(filepath.Base(remotePath), info.Size(), info.Size(), true)
		return fmt.Errorf("download copy: %w", err)
	}

	// 发送完成事件
	t.sendProgress(filepath.Base(remotePath), info.Size(), info.Size(), true)
	return nil
}

// sendProgress 发送传输进度事件（非阻塞）
func (t *Transfer) sendProgress(fileName string, bytes, total int64, done bool) {
	select {
	case t.uiChan <- event.UIEvent{
		TabID: t.tabID,
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

	mu       sync.Mutex
	lastSent time.Time
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
