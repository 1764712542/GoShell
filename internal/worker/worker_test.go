// Package worker_test 对各协议 worker 进行编译期接口契约校验。
// 使用外部测试包 (worker_test) 以便导入各协议实现包而不引入循环依赖：
// 实现包 (ssh/telnet/...) 依赖 worker 包，而本测试包同时依赖两者，
// 由于测试包是独立编译单元，不会构成包级循环。
package worker_test

import (
	"testing"

	"github.com/zhuyao/meatshell/internal/ftp"
	"github.com/zhuyao/meatshell/internal/localterminal"
	"github.com/zhuyao/meatshell/internal/mosh"
	"github.com/zhuyao/meatshell/internal/rlogin"
	"github.com/zhuyao/meatshell/internal/serial"
	"github.com/zhuyao/meatshell/internal/ssh"
	"github.com/zhuyao/meatshell/internal/telnet"
	"github.com/zhuyao/meatshell/internal/worker"
)

// TestWorkerInterface 确保各协议 worker 类型在编译期满足 worker.Worker 接口。
// 若任一实现缺少 Connect/Close/SendInput/Resize/IsConnected/SessionID 方法，
// 或方法签名不匹配，编译会直接失败——这是最有效的接口契约保护。
func TestWorkerInterface(t *testing.T) {
	// 编译期断言：*T 必须实现 worker.Worker 的全部方法
	var _ worker.Worker = (*ssh.Worker)(nil)
	var _ worker.Worker = (*telnet.Worker)(nil)
	var _ worker.Worker = (*ftp.Worker)(nil)
	var _ worker.Worker = (*mosh.Worker)(nil)
	var _ worker.Worker = (*rlogin.Worker)(nil)
	var _ worker.Worker = (*serial.Worker)(nil)
	var _ worker.Worker = (*localterminal.Worker)(nil)

	// 运行期额外确认接口集合非空
	var w worker.Worker
	impls := map[string]worker.Worker{
		"ssh":           (*ssh.Worker)(nil),
		"telnet":        (*telnet.Worker)(nil),
		"ftp":           (*ftp.Worker)(nil),
		"mosh":          (*mosh.Worker)(nil),
		"rlogin":        (*rlogin.Worker)(nil),
		"serial":        (*serial.Worker)(nil),
		"localterminal": (*localterminal.Worker)(nil),
	}
	for name, impl := range impls {
		w = impl
		if w == nil {
			t.Errorf("%s worker is nil", name)
		}
	}
	_ = w
}
