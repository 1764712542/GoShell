package ssh

import (
	"fmt"
	"io"
	"net"

	"github.com/zhuyao/meatshell/internal/event"
	"github.com/zhuyao/meatshell/internal/log"
)

// tunnelInfo 存储隧道的信息
type tunnelInfo struct {
	typ        string       // local / remote / dynamic
	listener   net.Listener // 本地或远程监听器
	localAddr  string       // 本地地址
	remoteAddr string       // 远端地址
}

// LocalForward 设置本地端口转发（SSH -L）。
// 在本地监听 localAddr，将连接通过 SSH 隧道转发到远端 remoteAddr。
func (w *Worker) LocalForward(localAddr, remoteAddr string) error {
	if w.client == nil {
		return fmt.Errorf("ssh client not connected")
	}

	listener, err := net.Listen("tcp", localAddr)
	if err != nil {
		return fmt.Errorf("local listen %s: %w", localAddr, err)
	}

	w.mu.Lock()
	w.tunnels = append(w.tunnels, &tunnelInfo{
		typ:        "local",
		listener:   listener,
		localAddr:  localAddr,
		remoteAddr: remoteAddr,
	})
	w.mu.Unlock()

	w.sendTunnelStatus("local", localAddr, remoteAddr, true, "")
	log.Info("local tunnel established", "local", localAddr, "remote", remoteAddr)

	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				break
			}
			go w.handleLocalForward(conn, remoteAddr)
		}
	}()

	return nil
}

// handleLocalForward 处理本地转发的单个连接
func (w *Worker) handleLocalForward(localConn net.Conn, remoteAddr string) {
	defer localConn.Close()

	remoteConn, err := w.client.Dial("tcp", remoteAddr)
	if err != nil {
		log.Warn("local tunnel dial remote failed", "remote", remoteAddr, "err", err)
		return
	}
	defer remoteConn.Close()

	pipe(localConn, remoteConn)
}

// RemoteForward 设置远程端口转发（SSH -R）。
// 在远端监听 remoteAddr，将连接通过 SSH 隧道转发到本地 localAddr。
func (w *Worker) RemoteForward(remoteAddr, localAddr string) error {
	if w.client == nil {
		return fmt.Errorf("ssh client not connected")
	}

	listener, err := w.client.Listen("tcp", remoteAddr)
	if err != nil {
		return fmt.Errorf("remote listen %s: %w", remoteAddr, err)
	}

	w.mu.Lock()
	w.tunnels = append(w.tunnels, &tunnelInfo{
		typ:        "remote",
		listener:   listener,
		localAddr:  localAddr,
		remoteAddr: remoteAddr,
	})
	w.mu.Unlock()

	w.sendTunnelStatus("remote", localAddr, remoteAddr, true, "")
	log.Info("remote tunnel established", "remote", remoteAddr, "local", localAddr)

	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				break
			}
			go w.handleRemoteForward(conn, localAddr)
		}
	}()

	return nil
}

// handleRemoteForward 处理远程转发的单个连接
func (w *Worker) handleRemoteForward(remoteConn net.Conn, localAddr string) {
	defer remoteConn.Close()

	localConn, err := net.Dial("tcp", localAddr)
	if err != nil {
		log.Warn("remote tunnel dial local failed", "local", localAddr, "err", err)
		return
	}
	defer localConn.Close()

	pipe(remoteConn, localConn)
}

// DynamicForward 设置动态端口转发（SSH -D），启动 SOCKS5 代理服务器。
// 在本地监听 localAddr，通过 SSH 隧道将 SOCKS5 请求转发到远端。
func (w *Worker) DynamicForward(localAddr string) error {
	if w.client == nil {
		return fmt.Errorf("ssh client not connected")
	}

	listener, err := net.Listen("tcp", localAddr)
	if err != nil {
		return fmt.Errorf("dynamic listen %s: %w", localAddr, err)
	}

	w.mu.Lock()
	w.tunnels = append(w.tunnels, &tunnelInfo{
		typ:       "dynamic",
		listener:  listener,
		localAddr: localAddr,
	})
	w.mu.Unlock()

	w.sendTunnelStatus("dynamic", localAddr, "", true, "")
	log.Info("dynamic tunnel (socks5) established", "local", localAddr)

	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				break
			}
			go w.handleSOCKS5(conn)
		}
	}()

	return nil
}

// handleSOCKS5 处理 SOCKS5 连接请求
func (w *Worker) handleSOCKS5(conn net.Conn) {
	defer conn.Close()

	// SOCKS5 握手：读取客户端支持的认证方法
	buf := make([]byte, 512)
	n, err := conn.Read(buf)
	if err != nil || n < 3 {
		return
	}
	if buf[0] != 0x05 { // SOCKS 版本号必须为 5
		return
	}

	// 响应：无需认证
	if _, err := conn.Write([]byte{0x05, 0x00}); err != nil {
		return
	}

	// 读取连接请求
	n, err = conn.Read(buf)
	if err != nil || n < 7 {
		return
	}
	if buf[0] != 0x05 || buf[1] != 0x01 { // 只支持 CONNECT 命令
		conn.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	// 解析目标地址
	var target string
	switch buf[3] { // 地址类型
	case 0x01: // IPv4
		if n < 10 {
			return
		}
		target = fmt.Sprintf("%d.%d.%d.%d:%d",
			buf[4], buf[5], buf[6], buf[7],
			int(buf[8])<<8|int(buf[9]))
	case 0x03: // 域名
		if n < 5 {
			return
		}
		domainLen := int(buf[4])
		if n < 5+domainLen+2 {
			return
		}
		domain := string(buf[5 : 5+domainLen])
		port := int(buf[5+domainLen])<<8 | int(buf[6+domainLen])
		target = fmt.Sprintf("%s:%d", domain, port)
	case 0x04: // IPv6
		if n < 22 {
			return
		}
		ip := net.IP(buf[4:20])
		port := int(buf[20])<<8 | int(buf[21])
		target = fmt.Sprintf("[%s]:%d", ip.String(), port)
	default:
		return
	}

	// 通过 SSH 隧道拨号到目标地址
	remoteConn, err := w.client.Dial("tcp", target)
	if err != nil {
		log.Warn("socks5 dial failed", "target", target, "err", err)
		// 返回连接失败响应
		conn.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	defer remoteConn.Close()

	// 返回连接成功响应
	localAddr := conn.LocalAddr().(*net.TCPAddr)
	resp := []byte{0x05, 0x00, 0x00, 0x01}
	bndAddr := localAddr.IP.To4()
	if bndAddr == nil {
		bndAddr = []byte{0, 0, 0, 0}
	}
	resp = append(resp, bndAddr...)
	resp = append(resp, byte(localAddr.Port>>8), byte(localAddr.Port))
	if _, err := conn.Write(resp); err != nil {
		return
	}

	// 双向转发数据
	pipe(conn, remoteConn)
}

// StopTunnels 停止所有隧道
func (w *Worker) StopTunnels() {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, t := range w.tunnels {
		t.listener.Close()
		w.sendTunnelStatus(t.typ, t.localAddr, t.remoteAddr, false, "")
	}
	w.tunnels = nil
	log.Info("all tunnels stopped")
}

// sendTunnelStatus 发送隧道状态事件到 UI
func (w *Worker) sendTunnelStatus(typ, localAddr, remoteAddr string, active bool, errMsg string) {
	select {
	case w.uiChan <- event.UIEvent{
		TabID: w.session.ID,
		Type:  event.EventTunnel,
		TunnelStatus: &event.TunnelStatus{
			Type:       typ,
			LocalAddr:  localAddr,
			RemoteAddr: remoteAddr,
			Active:     active,
			Error:      errMsg,
		},
	}:
	default:
		// UI 通道已满，丢弃事件以避免阻塞
	}
}

// pipe 在两个连接之间双向转发数据，任一方向断开后关闭双方
func pipe(a, b io.ReadWriteCloser) {
	done := make(chan struct{}, 2)

	go func() {
		io.Copy(a, b)
		done <- struct{}{}
	}()

	go func() {
		io.Copy(b, a)
		done <- struct{}{}
	}()

	<-done
	a.Close()
	b.Close()
}
