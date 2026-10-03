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
	"net"
	"os"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	boxService "github.com/sagernet/sing-box/adapter/service"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/service"
)

// 生成测试CA 生成测试用 CA 证书与私钥，返回 PEM 字节
func 生成测试CA(t *testing.T) (证书PEM []byte, 私钥PEM []byte) {
	t.Helper()
	私钥, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("生成私钥失败: %v", err)
	}
	模板 := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "sing-box MITM 测试 CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
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

// 写入临时文件 写入临时文件并返回路径
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

// 构造上下文 构造带 service 注册表的 context
func 构造上下文() context.Context {
	ctx := context.Background()
	ctx = service.ContextWith(ctx, boxService.NewRegistry())
	return ctx
}

// 获取测试日志器 获取 NOP 日志器
func 获取测试日志器() log.ContextLogger {
	return log.NewNOPFactory().Logger()
}

// TestNewService_未启用 验证未启用时不加载证书也能创建服务
func TestNewService_未启用(t *testing.T) {
	ctx := 构造上下文()
	svc, err := NewService(ctx, 获取测试日志器(), "mitm", option.MITMServiceOptions{Enabled: false})
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
	_, err := NewService(ctx, 获取测试日志器(), "mitm", option.MITMServiceOptions{Enabled: true})
	if err == nil {
		t.Fatal("缺少证书路径时应返回错误")
	}
}

// TestNewService_缺少私钥路径 验证缺少私钥时返回错误
func TestNewService_缺少私钥路径(t *testing.T) {
	ctx := 构造上下文()
	_, err := NewService(ctx, 获取测试日志器(), "mitm", option.MITMServiceOptions{
		Enabled: true,
		CA:      option.MITMCAOptions{Certificate: "/tmp/不存在的证书.pem"},
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
	svc, err := NewService(ctx, 获取测试日志器(), "mitm", option.MITMServiceOptions{
		Enabled: true,
		CA: option.MITMCAOptions{
			Certificate: 证书路径,
			PrivateKey:  私钥路径,
		},
		Match: option.MITMMatchOptions{
			DomainSuffix: []string{"example.com"},
		},
	})
	if err != nil {
		t.Fatalf("加载合法 CA 应成功: %v", err)
	}
	实际服务 := svc.(*Service)
	if 实际服务.根证书 == nil {
		t.Fatal("根证书不应为 nil")
	}
	if 实际服务.根证书.证书实体 == nil {
		t.Fatal("根证书实体不应为 nil")
	}
	if !实际服务.根证书.证书实体.IsCA {
		t.Error("根证书应为 CA 证书")
	}
	if 实际服务.匹配器 == nil {
		t.Fatal("匹配器不应为 nil")
	}
}

// TestNewService_非CA证书被拒绝 验证非 CA 证书会被拒绝
func TestNewService_非CA证书被拒绝(t *testing.T) {
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
	_, err := NewService(ctx, 获取测试日志器(), "mitm", option.MITMServiceOptions{
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
	svc, _ := NewService(ctx, 获取测试日志器(), "mitm", option.MITMServiceOptions{Enabled: false})
	scope := adapter.NewScope(ctx, 获取测试日志器())
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
	svc, _ := NewService(ctx, 获取测试日志器(), "mitm", option.MITMServiceOptions{Enabled: false})
	if FromContext(ctx) != nil {
		t.Error("未启用的服务不应注册到上下文")
	}
	证书PEM, 私钥PEM := 生成测试CA(t)
	证书路径 := 写入临时文件(t, 证书PEM)
	私钥路径 := 写入临时文件(t, 私钥PEM)
	ctx2 := 构造上下文()
	_, _ = NewService(ctx2, 获取测试日志器(), "mitm", option.MITMServiceOptions{
		Enabled: true,
		CA:      option.MITMCAOptions{Certificate: 证书路径, PrivateKey: 私钥路径},
	})
	if FromContext(ctx2) == nil {
		t.Error("启用的服务应能从上下文获取")
	}
	_ = svc
}

// Test域名匹配器_精确匹配 验证域名精确匹配
func Test域名匹配器_精确匹配(t *testing.T) {
	m := 新域名匹配器(option.MITMMatchOptions{
		Domain: []string{"example.com"},
	})
	if !m.匹配("example.com") {
		t.Error("example.com 应命中精确匹配")
	}
	if m.匹配("api.example.com") {
		t.Error("api.example.com 不应命中精确匹配")
	}
}

// Test域名匹配器_后缀匹配 验证域名后缀匹配
func Test域名匹配器_后缀匹配(t *testing.T) {
	m := 新域名匹配器(option.MITMMatchOptions{
		DomainSuffix: []string{"example.com"},
	})
	if !m.匹配("example.com") {
		t.Error("example.com 应命中后缀匹配")
	}
	if !m.匹配("api.example.com") {
		t.Error("api.example.com 应命中后缀匹配")
	}
	if m.匹配("notexample.com") {
		t.Error("notexample.com 不应命中后缀匹配")
	}
}

// Test证书缓存_LRU 验证证书缓存的 LRU 淘汰
func Test证书缓存_LRU(t *testing.T) {
	c := 新证书缓存(2)
	c.存入("a.com", nil)
	c.存入("b.com", nil)
	c.存入("c.com", nil) // 应淘汰 a.com
	if c.数量() != 2 {
		t.Errorf("缓存数量应为 2，实际: %d", c.数量())
	}
	if _, 命中 := c.获取("a.com"); 命中 {
		t.Error("a.com 应已被淘汰")
	}
	if _, 命中 := c.获取("b.com"); !命中 {
		t.Error("b.com 应仍在缓存中")
	}
}

// Test签发叶子证书 验证能正常签发叶子证书
func Test签发叶子证书(t *testing.T) {
	证书PEM, 私钥PEM := 生成测试CA(t)
	证书路径 := 写入临时文件(t, 证书PEM)
	私钥路径 := 写入临时文件(t, 私钥PEM)

	ctx := 构造上下文()
	svc, _ := NewService(ctx, 获取测试日志器(), "mitm", option.MITMServiceOptions{
		Enabled: true,
		CA:      option.MITMCAOptions{Certificate: 证书路径, PrivateKey: 私钥路径},
	})
	实际服务 := svc.(*Service)

	证书1, err := 实际服务.签发叶子证书("test.example.com")
	if err != nil {
		t.Fatalf("签发叶子证书失败: %v", err)
	}
	if 证书1 == nil {
		t.Fatal("叶子证书不应为 nil")
	}

	// 第二次应从缓存获取
	证书2, err := 实际服务.签发叶子证书("test.example.com")
	if err != nil {
		t.Fatalf("第二次签发应从缓存获取: %v", err)
	}
	if 证书1 != 证书2 {
		t.Error("同一域名应返回缓存中的证书实例")
	}
}

// Test读取客户端问候_SNI 验证能从 ClientHello 中提取 SNI
func Test读取客户端问候_SNI(t *testing.T) {
	// 构造一个最小的 TLS ClientHello（含 SNI=example.com）
	clientHello := 构造测试ClientHello("example.com", []string{"h2", "http/1.1"})
	连接 := &mockConn{数据: clientHello}

	信息, err := 读取客户端问候(连接)
	if err != nil {
		t.Fatalf("读取 ClientHello 失败: %v", err)
	}
	if 信息.SNI != "example.com" {
		t.Errorf("SNI 应为 example.com，实际: %s", 信息.SNI)
	}
	if len(信息.ALPN) != 2 || 信息.ALPN[0] != "h2" {
		t.Errorf("ALPN 应为 [h2 http/1.1]，实际: %v", 信息.ALPN)
	}
}

// mockConn 模拟 net.Conn，用于测试
type mockConn struct {
	数据 []byte
	偏移 int
}

func (m *mockConn) Read(p []byte) (int, error) {
	if m.偏移 >= len(m.数据) {
		return 0, errEOF
	}
	n := copy(p, m.数据[m.偏移:])
	m.偏移 += n
	return n, nil
}
func (m *mockConn) Write(p []byte) (int, error)         { return len(p), nil }
func (m *mockConn) Close() error                        { return nil }
func (m *mockConn) LocalAddr() net.Addr                 { return &net.TCPAddr{} }
func (m *mockConn) RemoteAddr() net.Addr                { return &net.TCPAddr{} }
func (m *mockConn) SetDeadline(t time.Time) error       { return nil }
func (m *mockConn) SetReadDeadline(t time.Time) error   { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error  { return nil }

var errEOF = eofError{}

type eofError struct{}

func (e eofError) Error() string   { return "EOF" }
func (e eofError) Timeout() bool   { return false }
func (e eofError) Temporary() bool { return false }

// 构造测试ClientHello 构造含指定 SNI 和 ALPN 的 TLS ClientHello
func 构造测试ClientHello(sni string, alpn []string) []byte {
	// 构造 extensions
	var extensions []byte

	// SNI extension (type=0)
	// 结构: type(2) + ext_data_len(2) + server_name_list_len(2) + name_type(1) + name_len(2) + name
	sniBytes := []byte(sni)
	sniListLen := 1 + 2 + len(sniBytes) // name_type + name_len + name
	sniExtLen := 2 + sniListLen         // list_len field + list data
	sniExt := make([]byte, 2+2+sniExtLen)
	// extension type
	sniExt[0] = 0x00
	sniExt[1] = 0x00
	// extension data length
	sniExt[2] = byte(sniExtLen >> 8)
	sniExt[3] = byte(sniExtLen)
	// server_name_list length
	sniExt[4] = byte(sniListLen >> 8)
	sniExt[5] = byte(sniListLen)
	// name_type = host_name
	sniExt[6] = 0x00
	// name length
	sniExt[7] = byte(len(sniBytes) >> 8)
	sniExt[8] = byte(len(sniBytes))
	copy(sniExt[9:], sniBytes)
	extensions = append(extensions, sniExt...)

	// ALPN extension (type=16)
	// 结构: type(2) + ext_data_len(2) + protocol_name_list_len(2) + protocol_list
	var alpnList []byte
	for _, p := range alpn {
		alpnList = append(alpnList, byte(len(p)))
		alpnList = append(alpnList, []byte(p)...)
	}
	alpnExtDataLen := 2 + len(alpnList) // list_len field + list data
	alpnExt := make([]byte, 2+2+alpnExtDataLen)
	alpnExt[0] = 0x00
	alpnExt[1] = 0x10
	alpnExt[2] = byte(alpnExtDataLen >> 8)
	alpnExt[3] = byte(alpnExtDataLen)
	alpnExt[4] = byte(len(alpnList) >> 8)
	alpnExt[5] = byte(len(alpnList))
	copy(alpnExt[6:], alpnList)
	extensions = append(extensions, alpnExt...)

	// 构造 ClientHello body
	// version(2) + random(32) + session_id(1+0) + cipher_suites(2+2) + compression(1+1) + extensions(2+...)
	body := make([]byte, 0, 34+1+4+2+2+len(extensions))
	body = append(body, 0x03, 0x03) // TLS 1.2
	body = append(body, make([]byte, 32)...) // random
	body = append(body, 0x00) // session_id length=0
	body = append(body, 0x00, 0x02, 0x13, 0x01) // cipher_suites: TLS_AES_128_GCM_SHA256
	body = append(body, 0x01, 0x00) // compression: null
	body = append(body, byte(len(extensions)>>8), byte(len(extensions)))
	body = append(body, extensions...)

	// handshake header: type(1)=ClientHello + length(3)
	handshake := make([]byte, 4+len(body))
	handshake[0] = 0x01
	handshake[1] = byte(len(body) >> 16)
	handshake[2] = byte(len(body) >> 8)
	handshake[3] = byte(len(body))
	copy(handshake[4:], body)

	// TLS record header: content_type(1)=handshake + version(2) + length(2)
	record := make([]byte, 5+len(handshake))
	record[0] = 0x16 // handshake
	record[1] = 0x03
	record[2] = 0x03
	record[3] = byte(len(handshake) >> 8)
	record[4] = byte(len(handshake))
	copy(record[5:], handshake)

	return record
}
