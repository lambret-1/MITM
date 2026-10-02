package mitm

import (
	"bufio"
	"io"
	"net"
	"net/http"

	E "github.com/sagernet/sing/common/exceptions"
)

// HTTP1处理器 HTTP/1.1 协议处理器
//
// 职责（参考 README 第 17 节）：
//   - 从 TLS 终止后的连接读取 HTTP/1.1 请求
//   - 执行请求重写
//   - 交回 Router 决策 outbound
//   - 转发到上游并读取响应
//   - 执行响应重写
//   - 返回给客户端
//
// 需要处理：Host、URL、Method、Header、Cookie、Content-Length、
// Transfer-Encoding、gzip/br/zstd、chunked、trailers、连接复用。
type HTTP1处理器 struct {
	服务 *Service
}

// 新建HTTP1处理器 创建 HTTP/1.1 处理器
func 新建HTTP1处理器(服务 *Service) *HTTP1处理器 {
	return &HTTP1处理器{服务: 服务}
}

// 处理连接 处理单个 HTTP/1.1 连接
//
// 支持持久连接（Keep-Alive），循环读取请求直到连接关闭。
func (h *HTTP1处理器) 处理连接(客户端连接 net.Conn) error {
	读取器 := bufio.NewReader(客户端连接)
	for {
		req, err := http.ReadRequest(读取器)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return E.Cause(err, "mitm: 读取 HTTP/1.1 请求失败")
		}

		// 后续阶段实现：
		// 1. 请求重写
		// 2. Router 决策
		// 3. 上游转发
		// 4. 响应重写
		// 5. 写回客户端

		_ = req
		// 骨架阶段直接关闭，避免挂起
		return E.New("mitm: HTTP/1.1 处理尚未实现（待 Phase 4）")
	}
}
