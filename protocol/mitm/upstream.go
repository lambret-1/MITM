package mitm

import (
	"context"
	"crypto/tls"
	"net"
	"time"

	"github.com/sagernet/sing-box/adapter"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
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

// 建立上游连接 通过 Router 与真实服务器建立连接并完成 TLS 握手
//
// 流程（参考 README 第 16 节、第 23 节）：
//  1. 构造 sing-box metadata：network=tcp，destination=域名:443
//  2. 创建 net.Pipe 管道对
//  3. 将服务端一端交给 Router.RouteConnectionEx，由 Router 根据 route 规则
//     （domain、domain_suffix、geosite、geoip、rule-set 等）选择 outbound
//  4. 客户端一端作为我们的上游 TCP 连接
//  5. 在其上做 TLS 握手，ServerName 为目标域名
//
// 关键：Destination 必须设置为域名而非原始 IP，这样 Router 才能按域名规则路由。
// MITM 不直接 net.Dial 绕过 sing-box Router，所有出站流量必须经过 Router 决策。
func (s *Service) 建立上游连接(ctx context.Context, 路由器 adapter.Router, 元数据 adapter.InboundContext, 域名 string, alpn []string) (net.Conn, error) {
	// 创建管道对：一端交给 Router，一端作为我们的上游连接
	上游客户端, 上游服务端 := net.Pipe()

	// 构造上游连接的 metadata：目标为域名 + 443 端口
	// 这样 Router 可以根据域名规则（domain、domain_suffix、geosite 等）选择 outbound
	// Inbound 标记为 "mitm"，便于 route 规则区分 MITM 解密后的流量
	上游元数据 := 元数据
	上游元数据.Inbound = "mitm"
	上游元数据.InboundType = "mitm"
	上游元数据.Destination = M.ParseSocksaddrHostPort(域名, 443)
	上游元数据.Network = "tcp"

	s.logger.DebugContext(ctx, "mitm: 上游路由决策，目标域名: ", 域名)

	// 在 goroutine 中让 Router 处理上游连接
	go 路由器.RouteConnectionEx(ctx, 上游服务端, 上游元数据, nil)

	// 上游 TLS 握手超时控制
	超时 := time.Duration(s.获取上游超时()) * time.Second
	握手上下文, 取消 := context.WithTimeout(ctx, 超时)
	defer 取消()

	// 在客户端一端做 TLS 握手
	return 建立上游TLSWithContext(握手上下文, 上游客户端, 域名, alpn)
}

// 建立上游TLSWithContext 在已建立的 TCP 连接上与上游服务器完成 TLS 握手（带超时）
func 建立上游TLSWithContext(ctx context.Context, 连接 net.Conn, 域名 string, alpn []string) (*tls.Conn, error) {
	tls配置 := &tls.Config{
		ServerName:         域名,
		NextProtos:         alpn,
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: false,
	}
	tls连接 := tls.Client(连接, tls配置)

	// 带超时的握手
	握手完成 := make(chan error, 1)
	go func() {
		握手完成 <- tls连接.Handshake()
	}()

	select {
	case err := <-握手完成:
		if err != nil {
			tls连接.Close()
			return nil, E.Cause(err, "mitm: 上游 TLS 握手失败: ", 域名)
		}
		return tls连接, nil
	case <-ctx.Done():
		tls连接.Close()
		return nil, E.New("mitm: 上游 TLS 握手超时: ", 域名)
	}
}
