package mitm

import (
	"net"
	"net/http"

	E "github.com/sagernet/sing/common/exceptions"
	"golang.org/x/net/http2"
)

// HTTP2处理器 HTTP/2 协议处理器
//
// 职责（参考 README 第 18 节）：
//   - 管理 HTTP/2 连接的多路复用流
//   - 处理 HEADERS、DATA、RST_STREAM、WINDOW_UPDATE、SETTINGS 帧
//   - 每个流独立执行请求重写、Router 决策、上游转发、响应重写
//
// 使用 golang.org/x/net/http2，不自行实现 framing。
type HTTP2处理器 struct {
	服务    *Service
	服务器  *http2.Server
}

// 新建HTTP2处理器 创建 HTTP/2 处理器
func 新建HTTP2处理器(服务 *Service) *HTTP2处理器 {
	return &HTTP2处理器{
		服务:   服务,
		服务器: &http2.Server{},
	}
}

// 处理连接 处理单个 HTTP/2 连接
//
// HTTP/2 连接包含多个并发流，每个流独立处理请求/响应。
func (h *HTTP2处理器) 处理连接(客户端连接 net.Conn) error {
	// 后续阶段实现：
	// 1. http2.Server.ServeConn 处理连接
	// 2. 每个流构造 http.Request
	// 3. 请求重写 + Router 决策 + 上游转发
	// 4. 响应重写 + 写回流

	_ = h.服务器
	_ = 客户端连接
	return E.New("mitm: HTTP/2 处理尚未实现（待 Phase 4）")
}

// 处理流 处理单个 HTTP/2 流
func (h *HTTP2处理器) 处理流(req *http.Request) (*http.Response, error) {
	// 后续阶段实现
	_ = req
	return nil, E.New("mitm: HTTP/2 流处理尚未实现")
}
