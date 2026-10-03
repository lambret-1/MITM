package mitm

import (
	"context"
	"io"
	"net"
	"sync"

	"github.com/sagernet/sing-box/adapter"
	N "github.com/sagernet/sing/common/network"
	E "github.com/sagernet/sing/common/exceptions"
)

// 拦截器接口 TUN 入站与 MITM 之间的抽象接口
//
// 参考 README 第 10 节，TUN 不需要知道 MITM 内部实现，
// 只需要判断是否需要拦截，以及调用拦截方法。
type 拦截器接口 interface {
	// 是否拦截 判断给定连接元数据是否需要 MITM 拦截
	ShouldIntercept(元数据 adapter.InboundContext) bool

	// 拦截 对 TCP 连接执行 MITM 拦截
	// 完成 TLS 终止后，将明文流量交回 Router 分流。
	// SNI 不匹配时自动回退到正常路由。
	Intercept(ctx context.Context, 连接 net.Conn, 元数据 adapter.InboundContext, 路由器 adapter.Router, 关闭回调 N.CloseHandlerFunc)
}

// 是否拦截 判断是否需要对该连接进行 MITM 拦截
//
// 判断逻辑：
//  1. MITM 服务已启用
//  2. 匹配器非空
//  3. 目标端口为 443（HTTPS）
//  4. 非 DNS 等特殊协议流量
//
// 实际 SNI 域名匹配在拦截流程中完成。
// ShouldIntercept 是否拦截：判断是否需要对该连接进行 MITM 拦截
func (s *Service) ShouldIntercept(元数据 adapter.InboundContext) bool {
	if !s.是否启用() {
		return false
	}
	if s.匹配器 == nil || s.匹配器.是否为空() {
		return false
	}
	if 元数据.Protocol != "" {
		return false
	}
	return 元数据.Destination.Port == 443
}

// 拦截 执行 MITM 拦截主流程
//
// 完整流程（参考 README 第 11 节）：
//  1. 读取 TLS ClientHello，提取 SNI 和 ALPN
//  2. 域名匹配器判断是否需要解密
//  3. 不匹配 → 用回退读取器包装连接，交回 Router 正常路由
//  4. 匹配 → 动态签发叶子证书
//  5. 与客户端建立 TLS（tls.Server）
//  6. 通过 Router 建立上游连接并完成上游 TLS
//  7. 双向转发明文流量（Phase 4 将替换为 HTTP 引擎）
// Intercept 拦截：执行 MITM 拦截主流程
func (s *Service) Intercept(ctx context.Context, 连接 net.Conn, 元数据 adapter.InboundContext, 路由器 adapter.Router, 关闭回调 N.CloseHandlerFunc) {
	if !s.是否启用() {
		路由器.RouteConnectionEx(ctx, 连接, 元数据, 关闭回调)
		return
	}

	s.logger.InfoContext(ctx, "mitm: 开始拦截连接，来源: ", 元数据.Source, " 目标: ", 元数据.Destination)

	// 步骤 1：读取 ClientHello，提取 SNI
	客户端问候, err := 读取客户端问候(连接)
	if err != nil {
		s.logger.DebugContext(ctx, "mitm: 读取 ClientHello 失败，关闭连接: ", err)
		连接.Close()
		if 关闭回调 != nil {
			关闭回调(err)
		}
		return
	}

	s.logger.DebugContext(ctx, "mitm: ClientHello SNI=", 客户端问候.SNI, " ALPN=", 客户端问候.ALPN)

	// 步骤 2-3：域名匹配，不匹配则回退
	if 客户端问候.SNI == "" || !s.匹配器.匹配(客户端问候.SNI) {
		s.logger.DebugContext(ctx, "mitm: 域名未匹配，回退正常路由: ", 客户端问候.SNI)
		回退连接 := 新建回退读取器(连接, 客户端问候.原始数据)
		路由器.RouteConnectionEx(ctx, 回退连接, 元数据, 关闭回调)
		return
	}

	s.logger.InfoContext(ctx, "mitm: 域名命中，开始 TLS 终止: ", 客户端问候.SNI)

	// 步骤 4-5：签发叶子证书并与客户端建立 TLS
	客户端回退连接 := 新建回退读取器(连接, 客户端问候.原始数据)
	tls连接, err := s.终止TLS(客户端回退连接, 客户端问候.SNI, 客户端问候.ALPN)
	if err != nil {
		s.logger.ErrorContext(ctx, "mitm: TLS 终止失败: ", err)
		连接.Close()
		if 关闭回调 != nil {
			关闭回调(err)
		}
		return
	}
	defer tls连接.Close()

	s.logger.InfoContext(ctx, "mitm: 客户端 TLS 握手完成，协议: ", tls连接.ConnectionState().NegotiatedProtocol)

	// 步骤 6-7：根据 ALPN 选择 HTTP 引擎处理解密后的流量
	协商协议 := tls连接.ConnectionState().NegotiatedProtocol
	var 处理错误 error
	if 协商协议 == "h2" {
		s.logger.DebugContext(ctx, "mitm: 使用 HTTP/2 引擎")
		处理器 := 新建HTTP2处理器(s, 路由器, 元数据)
		处理错误 = 处理器.处理连接(ctx, tls连接)
	} else {
		s.logger.DebugContext(ctx, "mitm: 使用 HTTP/1.1 引擎")
		处理器 := 新建HTTP1处理器(s, 路由器, 元数据)
		处理错误 = 处理器.处理连接(ctx, tls连接)
	}

	if 处理错误 != nil && 处理错误 != io.EOF {
		s.logger.DebugContext(ctx, "mitm: HTTP 处理结束: ", 处理错误)
	}

	if 关闭回调 != nil {
		关闭回调(nil)
	}
}

// 双向转发TLS 在客户端 TLS 连接和上游 TLS 连接之间双向转发数据
func 双向转发TLS(客户端连接 net.Conn, 上游连接 net.Conn) error {
	var 等待组 sync.WaitGroup
	等待组.Add(2)
	var 首次错误 error
	var 错误锁 sync.Mutex

	go func() {
		defer 等待组.Done()
		_, err := io.Copy(上游连接, 客户端连接)
		if err != nil {
			错误锁.Lock()
			if 首次错误 == nil {
				首次错误 = err
			}
			错误锁.Unlock()
		}
		if 写关闭, ok := 上游连接.(interface{ CloseWrite() error }); ok {
			写关闭.CloseWrite()
		}
	}()

	go func() {
		defer 等待组.Done()
		_, err := io.Copy(客户端连接, 上游连接)
		if err != nil {
			错误锁.Lock()
			if 首次错误 == nil {
				首次错误 = err
			}
			错误锁.Unlock()
		}
		if 写关闭, ok := 客户端连接.(interface{ CloseWrite() error }); ok {
			写关闭.CloseWrite()
		}
	}()

	等待组.Wait()
	return 首次错误
}

// 错误服务未启用 MITM 服务未启用时调用拦截方法
var 错误服务未启用 = E.New("mitm: 服务未启用")
