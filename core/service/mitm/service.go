package mitm

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"os"
	"sync"

	"github.com/sagernet/sing-box/adapter"
	boxService "github.com/sagernet/sing-box/adapter/service"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/service"
)

// RegisterService 将 MITM 服务注册到服务注册表
func RegisterService(registry *boxService.Registry) {
	boxService.Register[option.MITMServiceOptions](registry, C.TypeMITM, NewService)
}

// 编译期断言：Service 必须实现 adapter.Service 接口
var _ adapter.Service = (*Service)(nil)

// Service MITM 服务主体
// 负责加载根证书、维护域名匹配器、动态签发叶子证书，
// 并在后续阶段接入 TUN 拦截与 HTTP 解密。
type Service struct {
	boxService.Adapter

	// ctx 服务上下文，用于获取 Router 等全局依赖
	ctx context.Context
	// logger 带上下文的日志器
	logger log.ContextLogger
	// options MITM 服务配置
	options option.MITMServiceOptions

	// 根证书相关
	// 根证书锁，保护证书缓存并发安全
	证书锁 sync.RWMutex
	// 根证书（解析后的 tls.Certificate，含私钥）
	根证书 *tls.Certificate
	// 根证书的 x509 结构，用于签发叶子证书时设置 Issuer
	根证书实体 *x509.Certificate
	// 叶子证书缓存：域名 -> 已签发的 tls.Certificate
	叶子证书缓存 map[string]*tls.Certificate
}

// NewService 构造 MITM 服务
// 完成配置校验与根证书加载，不启动实际拦截逻辑（后续阶段接入）。
func NewService(ctx context.Context, logger log.ContextLogger, tag string, options option.MITMServiceOptions) (adapter.Service, error) {
	if !options.Enabled {
		// 未启用时仍创建服务实例，但不加载证书，避免无意义的文件 IO
		return &Service{
			Adapter:   boxService.NewAdapter(C.TypeMITM, tag),
			ctx:       ctx,
			logger:    logger,
			options:   options,
			叶子证书缓存: make(map[string]*tls.Certificate),
		}, nil
	}

	// 校验 CA 配置完整性
	if options.CA.Certificate == "" {
		return nil, E.New("mitm: 缺少根证书文件路径 (ca.certificate)")
	}
	if options.CA.PrivateKey == "" {
		return nil, E.New("mitm: 缺少根证书私钥文件路径 (ca.private_key)")
	}

	svc := &Service{
		Adapter:      boxService.NewAdapter(C.TypeMITM, tag),
		ctx:          ctx,
		logger:       logger,
		options:      options,
		叶子证书缓存:  make(map[string]*tls.Certificate),
	}

	// 加载并验证根证书
	if err := svc.加载根证书(); err != nil {
		return nil, E.Cause(err, "mitm: 加载根证书失败")
	}

	// 将自身注册到上下文，供后续 TUN 拦截层获取
	service.MustRegister[*Service](ctx, svc)

	logger.Info("mitm: 服务初始化完成，根证书已加载")
	return svc, nil
}

// 加载根证书 读取 PEM 格式的证书与私钥，验证其为合法 CA 证书
func (s *Service) 加载根证书() error {
	证书PEM, err := os.ReadFile(s.options.CA.Certificate)
	if err != nil {
		return E.Cause(err, "读取根证书文件失败: ", s.options.CA.Certificate)
	}

	私钥PEM, err := os.ReadFile(s.options.CA.PrivateKey)
	if err != nil {
		return E.Cause(err, "读取根证书私钥文件失败: ", s.options.CA.PrivateKey)
	}

	// 加载证书+私钥对
	证书对, err := tls.X509KeyPair(证书PEM, 私钥PEM)
	if err != nil {
		return E.Cause(err, "解析根证书与私钥失败")
	}

	// 解析证书实体以验证 CA 属性
	if len(证书对.Certificate) == 0 {
		return E.New("根证书中不包含任何证书")
	}
	证书实体, err := x509.ParseCertificate(证书对.Certificate[0])
	if err != nil {
		return E.Cause(err, "解析根证书实体失败")
	}
	if !证书实体.IsCA {
		return E.New("提供的证书不是 CA 证书 (IsCA=false)，无法用于签发叶子证书")
	}
	if (证书实体.KeyUsage & x509.KeyUsageCertSign) == 0 {
		return E.New("提供的 CA 证书缺少 KeyUsageCertSign 权限，无法签发证书")
	}

	s.根证书 = &证书对
	s.根证书实体 = 证书实体
	return nil
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
