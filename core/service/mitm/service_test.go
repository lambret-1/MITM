package mitm

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	boxService "github.com/sagernet/sing-box/adapter/service"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/service"
)

// 生成测试用 CA 证书与私钥，返回 PEM 字节
func 生成测试CA(t *testing.T) (证书PEM []byte, 私钥PEM []byte) {
	t.Helper()
	私钥, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("生成私钥失败: %v", err)
	}
	模板 := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "sing-box MITM 测试 CA"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		IsCA:         true,
		KeyUsage:     x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	证书DER, err := x509.CreateCertificate(rand.Reader, 模板, 模板, &私钥.PublicKey, 私钥)
	if err != nil {
		t.Fatalf("创建证书失败: %v", err)
	}
	证书PEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: 证书DER})
	私钥DER, err := x509.MarshalECPrivateKey(私钥)
	if err != nil {
		t.Fatalf("序列化私钥失败: %v", err)
	}
	私钥PEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: 私钥DER})
	return
}

// 写入临时文件并返回路径
func 写入临时文件(t *testing.T, 内容 []byte) string {
	t.Helper()
	文件, err := os.CreateTemp("", "mitm-test-*")
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	defer 文件.Close()
	if _, err := 文件.Write(内容); err != nil {
		t.Fatalf("写入临时文件失败: %v", err)
	}
	t.Cleanup(func() { os.Remove(文件.Name()) })
	return 文件.Name()
}

// 构造带 service 上下文的 context
func 构造上下文() context.Context {
	ctx := context.Background()
	ctx = service.ContextWith(ctx, boxService.NewRegistry())
	return ctx
}

// TestNewService_未启用 验证未启用时不加载证书也能创建服务
func TestNewService_未启用(t *testing.T) {
	ctx := 构造上下文()
	logger := log.NewNOPFactory().Logger()
	svc, err := NewService(ctx, logger, "mitm", option.MITMServiceOptions{Enabled: false})
	if err != nil {
		t.Fatalf("未启用时创建服务不应失败: %v", err)
	}
	if svc == nil {
		t.Fatal("服务实例不应为 nil")
	}
	if svc.Type() != "mitm" {
		t.Errorf("服务类型应为 mitm，实际: %s", svc.Type())
	}
	if svc.Tag() != "mitm" {
		t.Errorf("服务标签应为 mitm，实际: %s", svc.Tag())
	}
}

// TestNewService_缺少证书路径 验证缺少 CA 配置时返回错误
func TestNewService_缺少证书路径(t *testing.T) {
	ctx := 构造上下文()
	logger := log.NewNOPFactory().Logger()
	_, err := NewService(ctx, logger, "mitm", option.MITMServiceOptions{Enabled: true})
	if err == nil {
		t.Fatal("缺少证书路径时应返回错误")
	}
}

// TestNewService_缺少私钥路径 验证缺少私钥时返回错误
func TestNewService_缺少私钥路径(t *testing.T) {
	ctx := 构造上下文()
	logger := log.NewNOPFactory().Logger()
	_, err := NewService(ctx, logger, "mitm", option.MITMServiceOptions{
		Enabled: true,
		CA: option.MITMCAOptions{Certificate: "/tmp/不存在的证书.pem"},
	})
	if err == nil {
		t.Fatal("缺少私钥路径时应返回错误")
	}
}

// TestNewService_加载合法CA 验证合法 CA 证书能正常加载
func TestNewService_加载合法CA(t *testing.T) {
	证书PEM, 私钥PEM := 生成测试CA(t)
	证书路径 := 写入临时文件(t, 证书PEM)
	私钥路径 := 写入临时文件(t, 私钥PEM)

	ctx := 构造上下文()
	logger := log.NewNOPFactory().Logger()
	svc, err := NewService(ctx, logger, "mitm", option.MITMServiceOptions{
		Enabled: true,
		CA: option.MITMCAOptions{
			Certificate: 证书路径,
			PrivateKey:  私钥路径,
		},
	})
	if err != nil {
		t.Fatalf("加载合法 CA 应成功: %v", err)
	}
	实际服务 := svc.(*Service)
	if 实际服务.根证书 == nil {
		t.Fatal("根证书不应为 nil")
	}
	if 实际服务.根证书实体 == nil {
		t.Fatal("根证书实体不应为 nil")
	}
	if !实际服务.根证书实体.IsCA {
		t.Error("根证书应为 CA 证书")
	}
}

// TestNewService_非CA证书被拒绝 验证非 CA 证书会被拒绝
func TestNewService_非CA证书被拒绝(t *testing.T) {
	// 生成一个非 CA 的叶子证书
	私钥, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	模板 := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: "非CA证书"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  false,
		BasicConstraintsValid: true,
	}
	证书DER, _ := x509.CreateCertificate(rand.Reader, 模板, 模板, &私钥.PublicKey, 私钥)
	证书PEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: 证书DER})
	私钥DER, _ := x509.MarshalECPrivateKey(私钥)
	私钥PEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: 私钥DER})

	证书路径 := 写入临时文件(t, 证书PEM)
	私钥路径 := 写入临时文件(t, 私钥PEM)

	ctx := 构造上下文()
	logger := log.NewNOPFactory().Logger()
	_, err := NewService(ctx, logger, "mitm", option.MITMServiceOptions{
		Enabled: true,
		CA: option.MITMCAOptions{
			Certificate: 证书路径,
			PrivateKey:  私钥路径,
		},
	})
	if err == nil {
		t.Fatal("非 CA 证书应被拒绝")
	}
}

// TestStart_生命周期 验证 Start 方法在各阶段正常返回
func TestStart_生命周期(t *testing.T) {
	ctx := 构造上下文()
	logger := log.NewNOPFactory().Logger()
	svc, _ := NewService(ctx, logger, "mitm", option.MITMServiceOptions{Enabled: false})
	scope := adapter.NewScope(ctx, logger)
	for _, stage := range adapter.ListStartStages {
		if err := svc.Start(stage, scope); err != nil {
			t.Fatalf("Start 阶段 %v 不应失败: %v", stage, err)
		}
	}
}

// TestRegisterService 验证服务注册后能从注册表创建 options
func TestRegisterService(t *testing.T) {
	registry := boxService.NewRegistry()
	RegisterService(registry)
	类型列表 := registry.OptionTypes()
	找到 := false
	for _, typ := range 类型列表 {
		if typ == "mitm" {
			找到 = true
		}
	}
	if !找到 {
		t.Errorf("注册表中应包含 mitm 类型，实际: %v", 类型列表)
	}
	options, 存在 := registry.CreateOptions("mitm")
	if !存在 {
		t.Fatal("应能创建 mitm options")
	}
	if _, ok := options.(*option.MITMServiceOptions); !ok {
		t.Errorf("options 类型应为 *MITMServiceOptions，实际: %T", options)
	}
}

// TestFromContext 验证服务能从上下文获取
func TestFromContext(t *testing.T) {
	ctx := 构造上下文()
	logger := log.NewNOPFactory().Logger()
	svc, _ := NewService(ctx, logger, "mitm", option.MITMServiceOptions{Enabled: false})
	// 未启用时不会注册到上下文，应返回 nil
	if FromContext(ctx) != nil {
		t.Error("未启用的服务不应注册到上下文")
	}
	// 启用时会注册
	证书PEM, 私钥PEM := 生成测试CA(t)
	证书路径 := 写入临时文件(t, 证书PEM)
	私钥路径 := 写入临时文件(t, 私钥PEM)
	ctx2 := 构造上下文()
	_, _ = NewService(ctx2, logger, "mitm", option.MITMServiceOptions{
		Enabled: true,
		CA:      option.MITMCAOptions{Certificate: 证书路径, PrivateKey: 私钥路径},
	})
	if FromContext(ctx2) == nil {
		t.Error("启用的服务应能从上下文获取")
	}
	_ = svc
	_ = filepath.Join
}
