package mitm

import (
	"context"
	"net"

	M "github.com/sagernet/sing/common/metadata"
	E "github.com/sagernet/sing/common/exceptions"
)

// 拦截器接口 TUN 入站与 MITM 之间的抽象接口
//
// 参考 README 第 10 节，TUN 不需要知道 MITM 内部实现，
// 只需要判断是否需要拦截，以及调用拦截方法。
//
// 后续阶段在 protocol/tun/inbound.go 中接入此接口。
type 拦截器接口 interface {
	// 是否拦截 判断给定连接元数据是否需要 MITM 拦截
	是否拦截(元数据 M.Metadata) bool

	// 拦截 对 TCP 连接执行 MITM 拦截
	// 完成 TLS 终止、HTTP 解密、Router 分流后，将数据转发到上游。
	拦截(ctx context.Context, 连接 net.Conn, 元数据 M.Metadata) error
}

// 是否拦截 判断是否需要对该连接进行 MITM 拦截
//
// 判断逻辑：
//  1. MITM 服务已启用
//  2. 目标端口为 443（或配置的 HTTPS 端口）
//  3. 后续阶段：读取 ClientHello 后根据 SNI 匹配域名规则
//
// 当前阶段仅做基础判断，实际 SNI 匹配在拦截流程中完成。
func (s *Service) 是否拦截(元数据 M.Metadata) bool {
	if !s.是否启用() {
		return false
	}
	if s.匹配器 == nil || s.匹配器.是否为空() {
		return false
	}
	// 第一阶段仅标记 443 端口为候选，实际 SNI 匹配在拦截时进行
	return 元数据.Destination.Port == 443
}

// 拦截 执行 MITM 拦截主流程
//
// 完整流程（参考 README 第 11 节）：
//  1. 读取 TLS ClientHello，提取 SNI
//  2. 域名匹配器判断是否需要解密
//  3. 不匹配 → 回退到普通路由
//  4. 匹配 → 动态签发叶子证书
//  5. 与客户端建立 TLS（tls.Server）
//  6. 根据 ALPN 选择 HTTP/1.1 或 HTTP/2 引擎
//  7. 解密后请求交回 sing-box Router
//  8. Router 决定 outbound，建立上游 TLS
//  9. 双向转发（含 rewrite）
//
// 当前阶段为骨架，后续阶段逐步实现。
func (s *Service) 拦截(ctx context.Context, 连接 net.Conn, 元数据 M.Metadata) error {
	if !s.是否启用() {
		return 错误服务未启用
	}
	s.logger.DebugContext(ctx, "mitm: 开始拦截连接，目标: ", 元数据.Destination)

	// 后续阶段实现：
	// 1. 读取 ClientHello
	// 2. SNI 匹配
	// 3. 签发证书 + TLS 终止
	// 4. HTTP 引擎处理
	// 5. Router 分流 + 上游连接
	// 6. 双向转发

	_ = ctx
	_ = 连接
	_ = 元数据
	return nil
}

// 错误服务未启用 MITM 服务未启用时调用拦截方法
var 错误服务未启用 = E.New("mitm: 服务未启用")
