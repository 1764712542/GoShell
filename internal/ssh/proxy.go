package ssh

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"golang.org/x/net/proxy"

	"github.com/zhuyao/meatshell/internal/config"
)

// dialTimeout 是建立 TCP 连接的默认超时时间
const dialTimeout = 15 * time.Second

// dialViaProxy 根据代理配置建立到目标地址的 TCP 连接。
// 如果 proxyCfg 为 nil，则直连目标地址。
func dialViaProxy(proxyCfg *config.ProxyConfig, target string) (net.Conn, error) {
	if proxyCfg == nil {
		return net.DialTimeout("tcp", target, dialTimeout)
	}

	switch proxyCfg.Type {
	case "socks5":
		return dialViaSOCKS5(proxyCfg, target)
	case "http":
		return dialViaHTTP(proxyCfg, target)
	default:
		return nil, fmt.Errorf("unsupported proxy type: %s", proxyCfg.Type)
	}
}

// dialViaSOCKS5 通过 SOCKS5 代理拨号到目标地址
func dialViaSOCKS5(proxyCfg *config.ProxyConfig, target string) (net.Conn, error) {
	proxyAddr := net.JoinHostPort(proxyCfg.Host, strconv.Itoa(proxyCfg.Port))

	var auth *proxy.Auth
	if proxyCfg.Username != "" {
		auth = &proxy.Auth{
			User:     proxyCfg.Username,
			Password: proxyCfg.Password,
		}
	}

	dialer, err := proxy.SOCKS5("tcp", proxyAddr, auth, &net.Dialer{Timeout: dialTimeout})
	if err != nil {
		return nil, fmt.Errorf("create socks5 dialer: %w", err)
	}

	conn, err := dialer.Dial("tcp", target)
	if err != nil {
		return nil, fmt.Errorf("socks5 dial %s: %w", target, err)
	}

	return conn, nil
}

// dialViaHTTP 通过 HTTP CONNECT 代理拨号到目标地址
func dialViaHTTP(proxyCfg *config.ProxyConfig, target string) (net.Conn, error) {
	proxyAddr := net.JoinHostPort(proxyCfg.Host, strconv.Itoa(proxyCfg.Port))

	conn, err := net.DialTimeout("tcp", proxyAddr, dialTimeout)
	if err != nil {
		return nil, fmt.Errorf("dial http proxy %s: %w", proxyAddr, err)
	}

	// 构建 CONNECT 请求
	connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", target, target)
	if proxyCfg.Username != "" {
		cred := proxyCfg.Username + ":" + proxyCfg.Password
		encoded := base64.StdEncoding.EncodeToString([]byte(cred))
		connectReq += "Proxy-Authorization: Basic " + encoded + "\r\n"
	}
	connectReq += "\r\n"

	if _, err := conn.Write([]byte(connectReq)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("send connect request: %w", err)
	}

	// 读取 HTTP 响应，使用 bufio.Reader 避免丢失隧道数据
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: "CONNECT"})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("read connect response: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		conn.Close()
		return nil, fmt.Errorf("http connect failed: %s", resp.Status)
	}

	// 如果 bufio.Reader 缓存了额外数据，需要包装连接以保留这些数据
	if br.Buffered() > 0 {
		return &bufferedConn{Conn: conn, r: br}, nil
	}

	return conn, nil
}

// bufferedConn 包装 net.Conn 和 bufio.Reader，确保缓冲区数据不丢失
type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (b *bufferedConn) Read(p []byte) (int, error) {
	return b.r.Read(p)
}
