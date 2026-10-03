package mitm

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/protocol/mitm/rewrite"
)

// 解析测试CA 从 PEM 字节解析 CA 证书和私钥
func 解析测试CA(t *testing.T, 证书PEM []byte, 私钥PEM []byte) (*x509.Certificate, interface{}) {
	t.Helper()
	证书块, _ := pem.Decode(证书PEM)
	if 证书块 == nil {
		t.Fatal("解析证书 PEM 失败")
	}
	证书, err := x509.ParseCertificate(证书块.Bytes)
	if err != nil {
		t.Fatalf("解析证书失败: %v", err)
	}
	私钥块, _ := pem.Decode(私钥PEM)
	if 私钥块 == nil {
		t.Fatal("解析私钥 PEM 失败")
	}
	私钥, err := x509.ParseECPrivateKey(私钥块.Bytes)
	if err != nil {
		t.Fatalf("解析私钥失败: %v", err)
	}
	return 证书, 私钥
}

// Test集成_TLS终止_完整握手 测试完整的 TLS 终止流程
func Test集成_TLS终止_完整握手(t *testing.T) {
	证书PEM, 私钥PEM := 生成测试CA(t)
	ca证书, ca私钥 := 解析测试CA(t, 证书PEM, 私钥PEM)

	svc := &Service{
		ctx:     context.Background(),
		logger:  获取测试日志器(),
		options: option.MITMServiceOptions{Enabled: true},
		根证书: &根证书实例{
			证书对: tls.Certificate{
				Certificate: [][]byte{ca证书.Raw},
				PrivateKey:  ca私钥,
			},
			证书实体: ca证书,
		},
		叶子缓存: 新证书缓存(默认缓存容量),
	}

	客户端端, 服务端端 := net.Pipe()
	defer 客户端端.Close()
	defer 服务端端.Close()

	域名 := "test.example.com"

	服务端错误 := make(chan error, 1)
	go func() {
		tls连接, err := svc.终止TLS(服务端端, 域名, []string{"http/1.1"})
		if err != nil {
			服务端错误 <- err
			return
		}
		defer tls连接.Close()
		buf := make([]byte, 1024)
		n, err := tls连接.Read(buf)
		if err != nil && err != io.EOF {
			服务端错误 <- err
			return
		}
		tls连接.Write(buf[:n])
		服务端错误 <- nil
	}()

	客户端池 := x509.NewCertPool()
	客户端池.AddCert(ca证书)
	客户端TLS := tls.Client(客户端端, &tls.Config{
		ServerName: 域名,
		RootCAs:    客户端池,
		NextProtos: []string{"http/1.1"},
	})

	if err := 客户端TLS.Handshake(); err != nil {
		t.Fatalf("客户端 TLS 握手失败: %v", err)
	}

	if 客户端TLS.ConnectionState().NegotiatedProtocol != "http/1.1" {
		t.Errorf("ALPN 协商失败，期望 http/1.1，实际 %s", 客户端TLS.ConnectionState().NegotiatedProtocol)
	}

	验证证书 := 客户端TLS.ConnectionState().PeerCertificates[0]
	if err := 验证证书.CheckSignatureFrom(ca证书); err != nil {
		t.Errorf("证书不是由 MITM CA 签发: %v", err)
	}
	if 验证证书.Subject.CommonName != 域名 {
		t.Errorf("证书 CN 不匹配，期望 %s，实际 %s", 域名, 验证证书.Subject.CommonName)
	}

	测试数据 := []byte("hello mitm")
	if _, err := 客户端TLS.Write(测试数据); err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	回显 := make([]byte, 1024)
	n, err := 客户端TLS.Read(回显)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if string(回显[:n]) != string(测试数据) {
		t.Errorf("回显数据不匹配，期望 %s，实际 %s", 测试数据, 回显[:n])
	}

	客户端TLS.Close()

	select {
	case err := <-服务端错误:
		if err != nil {
			t.Errorf("服务端错误: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("服务端超时")
	}
}

// Test集成_重写引擎_请求头修改 测试重写引擎的请求头修改
func Test集成_重写引擎_请求头修改(t *testing.T) {
	配置 := option.MITMRewriteOptions{
		Enabled: true,
		Rules: []option.MITMRewriteRule{
			{
				DomainSuffix:  []string{"example.com"},
				RequestHeader: map[string]string{"X-Debug": "1", "X-Test": "mitm"},
			},
		},
	}
	引擎 := rewrite.NewEngine(配置)

	req := httptest.NewRequest("GET", "https://api.example.com/v1/data", nil)
	if err := 引擎.RewriteRequest(req); err != nil {
		t.Fatalf("重写请求失败: %v", err)
	}

	if req.Header.Get("X-Debug") != "1" {
		t.Errorf("X-Debug 头未设置，期望 1，实际 %s", req.Header.Get("X-Debug"))
	}
	if req.Header.Get("X-Test") != "mitm" {
		t.Errorf("X-Test 头未设置，期望 mitm，实际 %s", req.Header.Get("X-Test"))
	}
}

// Test集成_重写引擎_响应头和Body替换 测试响应头修改和 gzip Body 替换
func Test集成_重写引擎_响应头和Body替换(t *testing.T) {
	配置 := option.MITMRewriteOptions{
		Enabled:     true,
		MaxBodySize: 1024 * 1024,
		Rules: []option.MITMRewriteRule{
			{
				DomainSuffix:   []string{"example.com"},
				ResponseHeader: map[string]string{"X-Rewritten": "true"},
				BodyReplace:    []option.MITMBodyReplaceRule{{Find: "hello", Replace: "hi"}},
			},
		},
	}
	引擎 := rewrite.NewEngine(配置)

	原始内容 := "hello world, hello mitm"
	var gzipBuffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&gzipBuffer)
	gzipWriter.Write([]byte(原始内容))
	gzipWriter.Close()

	req := httptest.NewRequest("GET", "https://api.example.com/test", nil)
	resp := &http.Response{
		StatusCode: 200,
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(gzipBuffer.Bytes())),
		Request:    req,
	}
	resp.Header.Set("Content-Encoding", "gzip")
	resp.ContentLength = int64(gzipBuffer.Len())

	if err := 引擎.RewriteResponse(resp); err != nil {
		t.Fatalf("重写响应失败: %v", err)
	}

	if resp.Header.Get("X-Rewritten") != "true" {
		t.Errorf("X-Rewritten 头未设置")
	}

	响应体, _ := io.ReadAll(resp.Body)
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzipReader, err := gzip.NewReader(bytes.NewReader(响应体))
		if err != nil {
			t.Fatalf("创建 gzip 读取器失败: %v", err)
		}
		解压后, _ := io.ReadAll(gzipReader)
		响应体 = 解压后
	}

	if !strings.Contains(string(响应体), "hi world") {
		t.Errorf("Body 替换失败，期望包含 'hi world'，实际 %s", string(响应体))
	}
	if strings.Contains(string(响应体), "hello world") {
		t.Errorf("Body 替换失败，不应包含 'hello world'，实际 %s", string(响应体))
	}
}

// Test集成_配置解析_完整配置 测试完整的 MITM 配置解析
func Test集成_配置解析_完整配置(t *testing.T) {
	配置JSON := `{
		"enabled": true,
		"ca": {"certificate": "/tmp/ca.pem", "private_key": "/tmp/ca.key"},
		"match": {"domain_suffix": ["example.com"]},
		"rewrite": {
			"enabled": true,
			"max_body_size": 5242880,
			"rules": [
				{
					"domain_suffix": ["example.com"],
					"path_prefix": "/api/",
					"response_header": {"X-Debug": "1"},
					"body_replace": [{"find": "old", "replace": "new"}]
				}
			]
		},
		"on_error": "bypass",
		"upstream_timeout": 15
	}`

	var 配置 option.MITMServiceOptions
	if err := json.Unmarshal([]byte(配置JSON), &配置); err != nil {
		t.Fatalf("配置解析失败: %v", err)
	}

	if !配置.Enabled {
		t.Error("Enabled 应为 true")
	}
	if 配置.OnError != "bypass" {
		t.Errorf("OnError 应为 bypass，实际 %s", 配置.OnError)
	}
	if 配置.UpstreamTimeout != 15 {
		t.Errorf("UpstreamTimeout 应为 15，实际 %d", 配置.UpstreamTimeout)
	}
	if !配置.Rewrite.Enabled {
		t.Error("Rewrite.Enabled 应为 true")
	}
	if 配置.Rewrite.MaxBodySize != 5242880 {
		t.Errorf("MaxBodySize 应为 5242880，实际 %d", 配置.Rewrite.MaxBodySize)
	}
	if len(配置.Rewrite.Rules) != 1 {
		t.Fatalf("Rewrite.Rules 数量应为 1，实际 %d", len(配置.Rewrite.Rules))
	}
	规则 := 配置.Rewrite.Rules[0]
	if 规则.PathPrefix != "/api/" {
		t.Errorf("PathPrefix 应为 /api/，实际 %s", 规则.PathPrefix)
	}
	if len(规则.BodyReplace) != 1 {
		t.Fatalf("BodyReplace 数量应为 1，实际 %d", len(规则.BodyReplace))
	}
	if 规则.BodyReplace[0].Find != "old" || 规则.BodyReplace[0].Replace != "new" {
		t.Errorf("BodyReplace 不匹配")
	}
}

// Test集成_失败时绕过策略 测试 on_error 配置策略
func Test集成_失败时绕过策略(t *testing.T) {
	svcBypass := &Service{options: option.MITMServiceOptions{OnError: "bypass"}}
	if !svcBypass.是否失败时绕过() {
		t.Error("bypass 模式应返回 true")
	}

	svcBlock := &Service{options: option.MITMServiceOptions{OnError: "block"}}
	if svcBlock.是否失败时绕过() {
		t.Error("block 模式应返回 false")
	}

	svcDefault := &Service{options: option.MITMServiceOptions{}}
	if !svcDefault.是否失败时绕过() {
		t.Error("默认模式应返回 true（bypass）")
	}
}

// Test集成_上游超时 测试上游超时配置
func Test集成_上游超时(t *testing.T) {
	svcDefault := &Service{options: option.MITMServiceOptions{}}
	if svcDefault.获取上游超时() != 30 {
		t.Errorf("默认上游超时应为 30，实际 %d", svcDefault.获取上游超时())
	}

	svcCustom := &Service{options: option.MITMServiceOptions{UpstreamTimeout: 60}}
	if svcCustom.获取上游超时() != 60 {
		t.Errorf("自定义上游超时应为 60，实际 %d", svcCustom.获取上游超时())
	}
}
