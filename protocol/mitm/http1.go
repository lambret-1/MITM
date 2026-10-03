package mitm

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/sagernet/sing-box/adapter"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
)

// HTTP1处理器 HTTP/1.1 协议处理器
//
// 职责（参考 README 第 17 节）：
//   - 从 TLS 终止后的连接读取 HTTP/1.1 请求
//   - 根据 Host 头通过 Router 建立上游 TLS 连接
//   - 转发请求到上游并读取响应
//   - 返回给客户端，支持 Keep-Alive 持久连接
//   - 检测 WebSocket Upgrade，切换到双向流转发
//
// 处理：Host、URL、Method、Header、Cookie、Content-Length、
// Transfer-Encoding、chunked、trailers、连接复用。
type HTTP1处理器 struct {
	服务    *Service
	路由器  adapter.Router
	元数据  adapter.InboundContext
}

// 新建HTTP1处理器 创建 HTTP/1.1 处理器
func 新建HTTP1处理器(服务 *Service, 路由器 adapter.Router, 元数据 adapter.InboundContext) *HTTP1处理器 {
	return &HTTP1处理器{
		服务:   服务,
		路由器: 路由器,
		元数据: 元数据,
	}
}

// 处理连接 处理单个 HTTP/1.1 连接
//
// 支持持久连接（Keep-Alive），循环读取请求直到连接关闭或客户端要求关闭。
func (h *HTTP1处理器) 处理连接(ctx context.Context, 客户端连接 net.Conn) error {
	读取器 := bufio.NewReader(客户端连接)
	写入器 := bufio.NewWriter(客户端连接)

	for {
		// 设置读取超时，避免空闲连接挂起
		客户端连接.SetReadDeadline(time.Now().Add(120 * time.Second))

		req, err := http.ReadRequest(读取器)
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return nil
			}
			return E.Cause(err, "mitm: 读取 HTTP/1.1 请求失败")
		}
		客户端连接.SetReadDeadline(time.Time{})

		h.服务.logger.DebugContext(ctx, "mitm: HTTP/1.1 请求 ", req.Method, " ", req.Host, req.URL.Path)

		// 检测 WebSocket Upgrade
		if 是否WebSocketUpgrade(req) {
			h.服务.logger.InfoContext(ctx, "mitm: 检测到 WebSocket Upgrade，切换双向转发")
			return h.处理WebSocket(ctx, 客户端连接, 读取器, req)
		}

		// 处理单个请求
		err = h.处理请求(ctx, req, 读取器, 写入器)
		if err != nil {
			return err
		}

		// 判断是否保持连接
		if !h.保持连接(req) {
			return nil
		}
	}
}

// 处理请求 处理单个 HTTP/1.1 请求
func (h *HTTP1处理器) 处理请求(ctx context.Context, req *http.Request, 读取器 *bufio.Reader, 写入器 *bufio.Writer) error {
	// 从 Host 头提取域名
	域名 := h.提取域名(req)
	if 域名 == "" {
		return h.发送错误响应(写入器, http.StatusBadRequest, "缺少 Host 头")
	}

	// 请求重写（修改请求头）
	if 重写引擎 := h.服务.获取重写引擎(); 重写引擎 != nil {
		if err := 重写引擎.RewriteRequest(req); err != nil {
			h.服务.logger.WarnContext(ctx, "mitm: 请求重写失败: ", err)
		}
	}

	// 建立上游 TLS 连接
	上游连接, err := h.服务.建立上游连接(ctx, h.路由器, h.元数据, 域名, []string{"http/1.1"})
	if err != nil {
		h.服务.logger.ErrorContext(ctx, "mitm: 建立上游连接失败: ", err)
		return h.发送错误响应(写入器, http.StatusBadGateway, "上游连接失败")
	}
	defer 上游连接.Close()

	// 写入请求到上游
	if err := req.Write(上游连接); err != nil {
		return E.Cause(err, "mitm: 写入上游请求失败")
	}

	// 读取上游响应
	上游读取器 := bufio.NewReader(上游连接)
	resp, err := http.ReadResponse(上游读取器, req)
	if err != nil {
		return E.Cause(err, "mitm: 读取上游响应失败")
	}
	defer resp.Body.Close()

	h.服务.logger.DebugContext(ctx, "mitm: 上游响应 ", resp.Status)

	// 响应重写（修改响应头 + Body 替换）
	if 重写引擎 := h.服务.获取重写引擎(); 重写引擎 != nil {
		if err := 重写引擎.RewriteResponse(resp); err != nil {
			h.服务.logger.WarnContext(ctx, "mitm: 响应重写失败: ", err)
		}
	}

	// 写入响应到客户端
	if err := resp.Write(写入器); err != nil {
		return E.Cause(err, "mitm: 写入客户端响应失败")
	}
	if err := 写入器.Flush(); err != nil {
		return E.Cause(err, "mitm: 刷新客户端缓冲区失败")
	}

	return nil
}

// 处理WebSocket 处理 WebSocket 升级请求
//
// 握手完成后转为原始双向流转发（参考 README 第 20 节）。
func (h *HTTP1处理器) 处理WebSocket(ctx context.Context, 客户端连接 net.Conn, 读取器 *bufio.Reader, req *http.Request) error {
	域名 := h.提取域名(req)
	if 域名 == "" {
		return h.发送错误响应(bufio.NewWriter(客户端连接), http.StatusBadRequest, "缺少 Host 头")
	}

	// 建立上游连接（不做 TLS，因为 WebSocket 可能是 ws:// 或 wss://）
	// 对于 MITM 场景，客户端已经做了 TLS，上游也应该是 TLS
	上游连接, err := h.服务.建立上游连接(ctx, h.路由器, h.元数据, 域名, []string{"http/1.1"})
	if err != nil {
		return E.Cause(err, "mitm: WebSocket 上游连接失败")
	}
	defer 上游连接.Close()

	// 转发 Upgrade 请求到上游
	if err := req.Write(上游连接); err != nil {
		return E.Cause(err, "mitm: 转发 WebSocket 握手请求失败")
	}

	// 读取上游 101 响应
	上游读取器 := bufio.NewReader(上游连接)
	resp, err := http.ReadResponse(上游读取器, req)
	if err != nil {
		return E.Cause(err, "mitm: 读取 WebSocket 握手响应失败")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		// 非 101 响应，直接转发给客户端
		resp.Write(客户端连接)
		return nil
	}

	// 转发 101 响应给客户端
	if err := resp.Write(客户端连接); err != nil {
		return E.Cause(err, "mitm: 转发 WebSocket 握手响应失败")
	}

	h.服务.logger.InfoContext(ctx, "mitm: WebSocket 握手完成，开始双向转发")

	// 握手完成，转为原始双向流转发
	// 注意：读取器中可能还有已缓冲的数据，需要先排空
	if 读取器.Buffered() > 0 {
		缓冲数据 := make([]byte, 读取器.Buffered())
		读取器.Read(缓冲数据)
		上游连接.Write(缓冲数据)
	}

	return 双向转发TLS(客户端连接, 上游连接)
}

// 提取域名 从请求中提取目标域名
func (h *HTTP1处理器) 提取域名(req *http.Request) string {
	host := req.Host
	if host == "" {
		host = req.URL.Host
	}
	// 去掉端口
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}
	return host
}

// 保持连接 判断是否保持连接（Keep-Alive）
func (h *HTTP1处理器) 保持连接(req *http.Request) bool {
	连接头 := strings.ToLower(req.Header.Get("Connection"))
	if 连接头 == "close" {
		return false
	}
	// HTTP/1.1 默认保持连接
	return req.ProtoAtLeast(1, 1)
}

// 发送错误响应 向客户端发送 HTTP 错误响应
func (h *HTTP1处理器) 发送错误响应(写入器 *bufio.Writer, 状态码 int, 消息 string) error {
	resp := &http.Response{
		StatusCode: 状态码,
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(消息)),
	}
	resp.Header.Set("Content-Type", "text/plain; charset=utf-8")
	resp.Header.Set("Connection", "close")
	resp.ContentLength = int64(len(消息))
	if err := resp.Write(写入器); err != nil {
		return err
	}
	return 写入器.Flush()
}

// 编译期断言：确保 M 包被引用
var _ = M.Socksaddr{}
