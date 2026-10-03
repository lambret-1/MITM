package mitm

import (
	"crypto/tls"
	"net"

	E "github.com/sagernet/sing/common/exceptions"
)

// 终止TLS 与客户端建立 TLS 连接，完成 TLS 终止
//
// 流程（参考 README 第 15 节）：
//  1. 根据 SNI 域名动态签发叶子证书
//  2. 构造 tls.Config，设置证书和 ALPN
//  3. tls.Server 包装客户端连接
//  4. 握手完成后返回明文连接
//
// 参数：
//   - 客户端连接：原始 TCP 连接（已读取 ClientHello 后需要用回退读取器包装）
//   - 域名：SNI 提取的目标域名
//   - alpn：客户端支持的 ALPN 列表
func (s *Service) 终止TLS(客户端连接 net.Conn, 域名 string, alpn []string) (*tls.Conn, error) {
	if s.根证书 == nil {
		return nil, E.New("mitm: 根证书未加载，无法终止 TLS")
	}

	// 签发叶子证书
	叶子证书, err := s.签发叶子证书(域名)
	if err != nil {
		return nil, 包装错误(err, "签发叶子证书失败")
	}

	// 构造 TLS 配置
	tls配置 := &tls.Config{
		Certificates: []tls.Certificate{*叶子证书},
		NextProtos:   alpn,
		MinVersion:   tls.VersionTLS12,
	}

	// 包装为 TLS 服务端连接
	tls连接 := tls.Server(客户端连接, tls配置)

	// 执行握手
	if err := tls连接.Handshake(); err != nil {
		tls连接.Close()
		return nil, 包装错误(err, "TLS 握手失败: "+域名)
	}

	return tls连接, nil
}

// 协商ALPN 根据客户端 ALPN 列表和服务端支持，选择最终协议
//
// 优先级：h2 > http/1.1
// 客户端不支持 ALPN 时默认 http/1.1。
func 协商ALPN(客户端ALPN []string) string {
	支持集合 := make(map[string]struct{})
	for _, p := range 客户端ALPN {
		支持集合[p] = struct{}{}
	}
	if _, 支持 := 支持集合["h2"]; 支持 {
		return "h2"
	}
	if _, 支持 := 支持集合["http/1.1"]; 支持 {
		return "http/1.1"
	}
	// 默认回退到 HTTP/1.1
	return "http/1.1"
}
