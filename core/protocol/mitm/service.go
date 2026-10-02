package mitm

import (
	"context"
	"sync"

	"github.com/sagernet/sing-box/adapter"
	boxService "github.com/sagernet/sing-box/adapter/service"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/service"
)

// RegisterService 将 MITM 服务注册到服务注册表
// 在 include/registry.go 的 ServiceRegistry() 中被调用
func RegisterService(registry *boxService.Registry) {
	boxService.Register[option.MITMServiceOptions](registry, C.TypeMITM, NewService)
}

// 编译期断言：Service 必须实现 adapter.Service 接口
var _ adapter.Service = (*Service)(nil)

// Service MITM 服务主体
// 负责根证书管理、域名匹配、动态证书签发、TLS 终止、HTTP 解密与重写，
// 并将解密后的流量交回 sing-box Router 进行分流。
//
// 生命周期：
//   - NewService：加载 CA、初始化各子模块
//   - Start：启动拦截逻辑（后续阶段接入 TUN 后激活）
type Service struct {
	boxService.Adapter

	// ctx 服务上下文，用于获取 Router 等全局依赖
	ctx context.Context
	// logger 带上下文的日志器
	logger log.ContextLogger
	// options MITM 服务配置
	options option.MITMServiceOptions

	// 根证书相关（ca.go）
	证书锁     sync.RWMutex
	根证书     *根证书实例
	叶子缓存   *证书缓存

	// 域名匹配器（matcher.go）
	匹配器 *域名匹配器

	// 后续阶段模块占位
	// 拦截器 interceptor.go
	// TLS 终止 tls.go
	// 上游连接 upstream.go
	// Router 集成 router.go
	// HTTP/1.1 http1.go
	// HTTP/2 http2.go
	// WebSocket websocket.go
	// 重写引擎 rewrite/
}

// NewService 构造 MITM 服务
// 完成配置校验、根证书加载、各子模块初始化。
// 未启用时不加载证书，避免无意义的文件 IO。
func NewService(ctx context.Context, logger log.ContextLogger, tag string, options option.MITMServiceOptions) (adapter.Service, error) {
	svc := &Service{
		Adapter: boxService.NewAdapter(C.TypeMITM, tag),
		ctx:     ctx,
		logger:  logger,
		options: options,
		叶子缓存: 新证书缓存(默认缓存容量),
	}

	if !options.Enabled {
		return svc, nil
	}

	// 校验 CA 配置完整性
	if options.CA.Certificate == "" {
		return nil, 错误缺少证书路径
	}
	if options.CA.PrivateKey == "" {
		return nil, 错误缺少私钥路径
	}

	// 加载并验证根证书
	根证书, err := 加载根证书(options.CA)
	if err != nil {
		return nil, 包装错误(err, "加载根证书失败")
	}
	svc.根证书 = 根证书

	// 初始化域名匹配器
	svc.匹配器 = 新域名匹配器(options.Match)

	// 将自身注册到上下文，供 TUN 拦截层获取
	service.MustRegister[*Service](ctx, svc)

	logger.Info("mitm: 服务初始化完成，根证书已加载")
	return svc, nil
}

// Start 实现 adapter.Lifecycle 接口
// MITM 服务在第一阶段仅完成初始化，实际拦截逻辑在后续阶段接入 TUN 后启动。
func (s *Service) Start(stage adapter.StartStage, scope *adapter.Scope) error {
	if stage != adapter.StartStateStart {
		return nil
	}
	if !s.options.Enabled {
		s.logger.Info("mitm: 服务未启用，跳过启动")
		return nil
	}
	s.logger.Info("mitm: 服务已启动")
	return nil
}

// FromContext 从上下文中获取 MITM 服务实例
// 供 TUN 入站等模块调用，判断是否需要拦截流量。
func FromContext(ctx context.Context) *Service {
	return service.FromContext[*Service](ctx)
}

// 是否启用 返回 MITM 服务是否已启用
func (s *Service) 是否启用() bool {
	return s.options.Enabled
}
