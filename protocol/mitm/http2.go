package mitm

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"

	"github.com/sagernet/sing-box/adapter"
	E "github.com/sagernet/sing/common/exceptions"
	"golang.org/x/net/http2"
)

// HTTP2处理器 HTTP/2 协议处理器
//
// 职责（参考 README 第 18 节）：
//   - 使用 golang.org/x/net/http2 管理多路复用流
//   - 每个流独立处理请求：Router 决策 → 上游转发 → 响应返回
//   - 保持 HPACK、HEADERS、DATA、RST_STREAM、WINDOW_UPDATE、SETTINGS 帧处理
//
// 不自行实现 HTTP/2 framing，完全依赖标准库扩展。
type HTTP2处理器 struct {
	服务    *Service
	路由器  adapter.Router
	元数据  adapter.InboundContext
	服务器  *http2.Server
}

// 新建HTTP2处理器 创建 HTTP/2 处理器
func 新建HTTP2处理器(服务 *Service, 路由器 adapter.Router, 元数据 adapter.InboundContext) *HTTP2处理器 {
	return &HTTP2处理器{
		服务:   服务,
		路由器: 路由器,
		元数据: 元数据,
		服务器: &http2.Server{},
	}
}

// 处理连接 处理单个 HTTP/2 连接
//
// HTTP/2 连接包含多个并发流，http2.Server 内部管理流生命周期，
// 每个流通过 Handler 独立处理请求/响应。
func (h *HTTP2处理器) 处理连接(ctx context.Context, 客户端连接 net.Conn) error {
	处理器 := &http2请求处理器{
		服务:   h.服务,
		路由器: h.路由器,
		元数据: h.元数据,
		上下文: ctx,
	}

	h.服务器.ServeConn(客户端连接, &http2.ServeConnOpts{
		Context: ctx,
		Handler: 处理器,
	})
	return nil
}

// http2请求处理器 实现 http.Handler，处理每个 HTTP/2 流
type http2请求处理器 struct {
	服务    *Service
	路由器  adapter.Router
	元数据  adapter.InboundContext
	上下文  context.Context
}

// ServeHTTP 处理单个 HTTP/2 流的请求
func (h *http2请求处理器) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	h.服务.logger.DebugContext(h.上下文, "mitm: HTTP/2 流请求 ", req.Method, " ", req.Host, req.URL.Path)

	// 从 Host 提取域名
	域名 := req.Host
	if 域名 == "" {
		域名 = req.URL.Host
	}
	if idx := lastIndexByte(域名, ':'); idx != -1 {
		域名 = 域名[:idx]
	}

	if 域名 == "" {
		http.Error(w, "缺少 Host 头", http.StatusBadRequest)
		return
	}

	// 请求重写（修改请求头）
	if 重写引擎 := h.服务.获取重写引擎(); 重写引擎 != nil {
		if err := 重写引擎.RewriteRequest(req); err != nil {
			h.服务.logger.WarnContext(h.上下文, "mitm: HTTP/2 请求重写失败: ", err)
		}
	}

	// 建立上游 TLS 连接（HTTP/2）
	上游连接, err := h.服务.建立上游连接(h.上下文, h.路由器, h.元数据, 域名, []string{"h2", "http/1.1"})
	if err != nil {
		h.服务.logger.ErrorContext(h.上下文, "mitm: HTTP/2 上游连接失败: ", err)
		http.Error(w, "上游连接失败", http.StatusBadGateway)
		return
	}
	defer 上游连接.Close()

	// 使用 http2.Client 发送请求
	上游传输 := &http2.Transport{
		DialTLSContext: func(ctx context.Context, network string, addr string, cfg *tls.Config) (net.Conn, error) {
			// 已经建立了 TLS 连接，直接返回
			return 上游连接, nil
		},
		AllowHTTP: false,
	}

	上游客户端 := &http.Client{
		Transport: 上游传输,
	}

	// 构造上游请求（复制必要字段）
	上游请求 := req.Clone(h.上下文)
	上游请求.URL.Scheme = "https"
	上游请求.URL.Host = req.Host
	上游请求.RequestURI = ""

	// 发送请求
	resp, err := 上游客户端.Do(上游请求)
	if err != nil {
		h.服务.logger.ErrorContext(h.上下文, "mitm: HTTP/2 上游请求失败: ", err)
		http.Error(w, "上游请求失败", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// 响应重写（修改响应头 + Body 替换）
	if 重写引擎 := h.服务.获取重写引擎(); 重写引擎 != nil {
		if err := 重写引擎.RewriteResponse(resp); err != nil {
			h.服务.logger.WarnContext(h.上下文, "mitm: HTTP/2 响应重写失败: ", err)
		}
	}

	// 复制响应头
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)

	// 复制响应体
	buffer := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			if _, werr := w.Write(buffer[:n]); werr != nil {
				return
			}
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
		if err != nil {
			break
		}
	}
}

// lastIndexByte 返回字符在字符串中最后一次出现的位置
func lastIndexByte(s string, c byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == c {
			return i
		}
	}
	return -1
}

// 编译期断言
var _ http.Handler = (*http2请求处理器)(nil)
var _ = E.New
