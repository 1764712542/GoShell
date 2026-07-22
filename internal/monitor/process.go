package monitor

import (
	"context"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/zhuyao/meatshell/internal/event"
	"github.com/zhuyao/meatshell/internal/log"
)

// ProcessMonitor 通过 SSH 采集远端进程列表
type ProcessMonitor struct {
	client *ssh.Client
	uiChan chan event.UIEvent
	tabID  string
	cancel context.CancelFunc
}

// NewProcessMonitor 创建进程监控器
func NewProcessMonitor(client *ssh.Client, uiChan chan event.UIEvent, tabID string) *ProcessMonitor {
	return &ProcessMonitor{
		client: client,
		uiChan: uiChan,
		tabID:  tabID,
	}
}

// Start 启动进程列表采集循环，interval 为采样间隔
func (pm *ProcessMonitor) Start(ctx context.Context, interval time.Duration) {
	ctx, pm.cancel = context.WithCancel(ctx)
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// 首次采集
	pm.listAndSend()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pm.listAndSend()
		}
	}
}

// Stop 停止采集循环
func (pm *ProcessMonitor) Stop() {
	if pm.cancel != nil {
		pm.cancel()
	}
}

// listAndSend 执行一次进程列表采集并发送事件。
// 使用非阻塞发送，避免在关闭过程中阻塞 goroutine。
func (pm *ProcessMonitor) listAndSend() {
	processes, err := pm.List()
	if err != nil {
		log.Warn("process monitor list failed", "err", err)
		// 采集失败时发送空列表，不中断循环
		select {
		case pm.uiChan <- event.UIEvent{
			TabID:     pm.tabID,
			Type:      event.EventProcess,
			Processes: []event.ProcessEntry{},
		}:
		default:
			// 通道已满或无人接收，丢弃事件
		}
		return
	}

	// 转换为 event.ProcessEntry
	entries := make([]event.ProcessEntry, 0, len(processes))
	for i := range processes {
		entries = append(entries, processes[i].ToProcessEntry())
	}

	select {
	case pm.uiChan <- event.UIEvent{
		TabID:     pm.tabID,
		Type:      event.EventProcess,
		Processes: entries,
	}:
	default:
		// 通道已满或无人接收，丢弃事件
	}
}

// runCommand 在远端执行命令并返回合并输出
func (pm *ProcessMonitor) runCommand(cmd string) ([]byte, error) {
	session, err := pm.client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()
	return session.CombinedOutput(cmd)
}

// List 执行单次采集，返回按 CPU 占用排序的进程列表
func (pm *ProcessMonitor) List() ([]ProcessInfo, error) {
	// ps aux 输出字段：USER PID %CPU %MEM VSZ RSS TTY STAT START TIME COMMAND
	// --sort=-%cpu 按CPU降序，head -21 取前20个进程（首行为标题）
	out, err := pm.runCommand("ps aux --sort=-%cpu | head -21")
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) <= 1 {
		return []ProcessInfo{}, nil
	}

	processes := make([]ProcessInfo, 0, len(lines)-1)
	// 跳过第一行标题
	for i := 1; i < len(lines); i++ {
		line := lines[i]
		fields := strings.Fields(line)
		// 至少需要 11 个字段：USER PID %CPU %MEM VSZ RSS TTY STAT START TIME COMMAND
		if len(fields) < 11 {
			continue
		}

		pid, _ := strconv.Atoi(fields[1])
		cpu, _ := strconv.ParseFloat(fields[2], 64)
		mem, _ := strconv.ParseFloat(fields[3], 64)
		// COMMAND 可能包含空格，将剩余字段合并
		command := strings.Join(fields[10:], " ")

		processes = append(processes, ProcessInfo{
			PID:     pid,
			User:    fields[0],
			CPU:     cpu,
			Mem:     mem,
			Command: command,
		})
	}

	return processes, nil
}
