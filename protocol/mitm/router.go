package mitm

import (
	"context"
	"net"
	"net/http"

	"github.com/sagernet/sing-box/adapter"
	C "github.com/sagernet/sing-box/constant"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/service"
	E "github.com/sagernet/sing/common/exceptions"
)

// 获取Router 从上下文中获取 sing-box Router
//
// 核心原则（参考 README 第 9、23、24 节）：
// MITM 永远不要自己创建一套 Router，
// 必须使用 sing-box 现有的 Router 进行分流决策。
func (s *Service) 获取Router() adapter.Router {
	return service.FromContext[adapter.Router](s.ctx)
}

// 构造入站上下文 根据 HTTP 请求构造 sing-box 入站上下文
//
// 字段（参考 README 第 25 节）：
//   - Inbound = "mitm"（标记流量来源，便于 route 规则区分）
//   - InboundType = "mitm"
//   - Network = tcp
//   - Source = 客户端地址
//   - Destination = 服务器地址（域名:443）
//
// 这样 Router 可以根据域名规则（domain、domain_suffix、geosite 等）选择 outbound。
func 构造入站上下文(req *http.Request, 源地址 M.Socksaddr) adapter.InboundContext {
	目标地址 := M.ParseSocksaddrHostPort(req.URL.Hostname(), 443)
	return adapter.InboundContext{
		Inbound:     "mitm",
		InboundType: C.TypeMITM,
		Network:     N.NetworkTCP,
		Source:      源地址,
		Destination: 目标地址,
	}
}

// 路由连接 将解密后的连接交给 sing-box Router 决策并转发
//
// 流程（参考 README 第 23 节）：
//  1. 从 HTTP 请求构造 InboundContext
//  2. 调用 Router.RouteConnection
//  3. Router 根据 route.rules 匹配 outbound
//  4. Router 自动建立 outbound 连接并双向转发
//
// MITM 不需要理解 rule-set、DNS rule、geoip、geosite、selector、urltest，
// 全部由 Router 处理。
//
// 注意：此方法用于原始 TCP 连接级别的路由。
// HTTP 引擎中使用 建立上游连接 方法，在其内部通过 net.Pipe + Router 建立连接。
func (s *Service) 路由连接(ctx context.Context, 连接 net.Conn, req *http.Request, 源地址 M.Socksaddr) error {
	router := s.获取Router()
	if router == nil {
		return E.New("mitm: Router 未在上下文中注册")
	}

	入站上下文 := 构造入站上下文(req, 源地址)
	return router.RouteConnection(ctx, 连接, 入站上下文)
}
