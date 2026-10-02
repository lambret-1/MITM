package mitm

import (
	"context"
	"crypto/tls"
	"net"

	E "github.com/sagernet/sing/common/exceptions"
)

// 上游连接信息 上游连接的元数据
type 上游连接信息 struct {
	// 连接 与上游服务器的 TCP 连接（可能经过 Router 的 outbound）
	连接 net.Conn
	// TLS连接 包装后的 TLS 连接（如果上游是 HTTPS）
	TLS连接 *tls.Conn
	// 协议 协商后的应用层协议（h2 / http/1.1）
	协议 string
}

// 建立上游连接 与真实服务器建立连接并完成 TLS 握手
//
// 注意（参考 README 第 16 节）：
// MITM 不应该直接 net.Dial 绕过 sing-box Router。
// 正确流程是将解密后的 HTTP 请求交回 Router，
// 由 Router 决定 outbound，再通过 outbound 建立连接。
//
// 当前阶段为骨架，后续阶段接入 Router 后实现。
func (s *Service) 建立上游连接(ctx context.Context, 域名 string, 端口 uint16) (*上游连接信息, error) {
	// 后续阶段：
	// 1. 构造 sing-box metadata（domain, port, network=tcp）
	// 2. 调用 Router.RouteConnection 获取 outbound
	// 3. 通过 outbound 建立 TCP 连接
	// 4. 包装 TLS 客户端，设置 ServerName
	// 5. 握手并返回

	_ = ctx
	_ = 域名
	_ = 端口
	return nil, E.New("mitm: 上游连接尚未实现（待 Router 集成阶段）")
}

// 建立上游TLS 在已建立的 TCP 连接上与上游服务器完成 TLS 握手
func 建立上游TLS(连接 net.Conn, 域名 string, alpn []string) (*tls.Conn, error) {
	tls配置 := &tls.Config{
		ServerName: 域名,
		NextProtos: alpn,
		MinVersion: tls.VersionTLS12,
	}
	tls连接 := tls.Client(连接, tls配置)
	if err := tls连接.Handshake(); err != nil {
		tls连接.Close()
		return nil, E.Cause(err, "mitm: 上游 TLS 握手失败: ", 域名)
	}
	return tls连接, nil
}
