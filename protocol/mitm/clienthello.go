package mitm

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"time"

	E "github.com/sagernet/sing/common/exceptions"
)

// 客户端问候信息 从 TLS ClientHello 中提取的信息
type 客户端问候信息 struct {
	// SNI 服务器名称指示（Server Name Indication）
	SNI string
	// ALPN 应用层协议协商列表（如 h2, http/1.1）
	ALPN []string
	// 原始数据 完整的 ClientHello 原始字节
	原始数据 []byte
}

// 读取客户端问候 从 TCP 连接中读取并解析 TLS ClientHello
//
// 读取流程（参考 README 第 12 节）：
//  1. 读取 TLS 记录头（5 字节）
//  2. 读取 handshake 头（4 字节）
//  3. 解析 ClientHello 扩展，提取 SNI 和 ALPN
//
// 不匹配 TLS 协议时返回错误，调用方应回退到普通路由。
func 读取客户端问候(连接 net.Conn) (*客户端问候信息, error) {
	// 读取 TLS 记录头
	记录头 := make([]byte, 5)
	if _, err := io.ReadFull(连接, 记录头); err != nil {
		return nil, E.Cause(err, "读取 TLS 记录头失败")
	}

	// 校验 ContentType = 22 (Handshake)
	if 记录头[0] != 0x16 {
		return nil, E.New("非 TLS Handshake 记录，类型: ", 记录头[0])
	}

	// 读取 handshake 数据长度
	握手长度 := binary.BigEndian.Uint16(记录头[3:5])
	if 握手长度 == 0 {
		return nil, E.New("TLS Handshake 数据为空")
	}

	// 读取 handshake 数据
	握手数据 := make([]byte, 握手长度)
	if _, err := io.ReadFull(连接, 握手数据); err != nil {
		return nil, E.Cause(err, "读取 TLS Handshake 数据失败")
	}

	// 校验 HandshakeType = 1 (ClientHello)
	if 握手数据[0] != 0x01 {
		return nil, E.New("非 ClientHello，类型: ", 握手数据[0])
	}

	// 组合完整 ClientHello（记录头 + 握手数据）
	完整数据 := append(记录头, 握手数据...)

	// 解析扩展提取 SNI 和 ALPN
	sni, alpn := 解析客户端问候扩展(握手数据[4:]) // 跳过 handshake header(4字节)

	return &客户端问候信息{
		SNI:     sni,
		ALPN:    alpn,
		原始数据: 完整数据,
	}, nil
}

// 解析客户端问候扩展 从 ClientHello body 中解析 SNI 和 ALPN 扩展
func 解析客户端问候扩展(body []byte) (sni string, alpn []string) {
	if len(body) < 34 {
		return "", nil
	}
	偏移 := 0

	// 跳过 client_version(2) + random(32)
	偏移 += 34

	// 跳过 session_id
	if 偏移 >= len(body) {
		return "", nil
	}
	会话ID长度 := int(body[偏移])
	偏移 += 1 + 会话ID长度

	// 跳过 cipher_suites
	if 偏移+2 > len(body) {
		return "", nil
	}
	密码套件长度 := int(binary.BigEndian.Uint16(body[偏移 : 偏移+2]))
	偏移 += 2 + 密码套件长度

	// 跳过 compression_methods
	if 偏移+1 > len(body) {
		return "", nil
	}
	压缩方法长度 := int(body[偏移])
	偏移 += 1 + 压缩方法长度

	// 解析 extensions
	if 偏移+2 > len(body) {
		return "", nil
	}
	扩展总长度 := int(binary.BigEndian.Uint16(body[偏移 : 偏移+2]))
	偏移 += 2
	扩展结束 := 偏移 + 扩展总长度
	if 扩展结束 > len(body) {
		扩展结束 = len(body)
	}

	for 偏移+4 <= 扩展结束 {
		扩展类型 := binary.BigEndian.Uint16(body[偏移 : 偏移+2])
		扩展长度 := int(binary.BigEndian.Uint16(body[偏移+2 : 偏移+4]))
		偏移 += 4
		if 偏移+扩展长度 > 扩展结束 {
			break
		}
		扩展数据 := body[偏移 : 偏移+扩展长度]

		switch 扩展类型 {
		case 0x0000: // server_name
			sni = 解析SNI扩展(扩展数据)
		case 0x0010: // application_layer_protocol_negotiation
			alpn = 解析ALPN扩展(扩展数据)
		}
		偏移 += 扩展长度
	}
	return sni, alpn
}

// 解析SNI扩展 解析 server_name 扩展，返回第一个主机名
func 解析SNI扩展(数据 []byte) string {
	if len(数据) < 5 {
		return ""
	}
	// 跳过 server_name_list 长度(2) + name_type(1) + name 长度(2)
	名称长度 := int(binary.BigEndian.Uint16(数据[3:5]))
	if 5+名称长度 > len(数据) {
		return ""
	}
	return string(数据[5 : 5+名称长度])
}

// 解析ALPN扩展 解析 ALPN 扩展，返回协议列表
func 解析ALPN扩展(数据 []byte) []string {
	if len(数据) < 2 {
		return nil
	}
	协议列表长度 := int(binary.BigEndian.Uint16(数据[0:2]))
	偏移 := 2
	结束 := 偏移 + 协议列表长度
	if 结束 > len(数据) {
		结束 = len(数据)
	}
	var 协议列表 []string
	for 偏移 < 结束 {
		协议长度 := int(数据[偏移])
		偏移++
		if 偏移+协议长度 > 结束 {
			break
		}
		协议列表 = append(协议列表, string(数据[偏移:偏移+协议长度]))
		偏移 += 协议长度
	}
	return 协议列表
}

// 回退读取器 将已读取的 ClientHello 数据重新包装为可读流
// 用于 MITM 不匹配时将原始数据交回正常路由。
type 回退读取器 struct {
	剩余数据 *bytes.Reader
	底层连接 net.Conn
}

// 新建回退读取器 创建回退读取器，将已读取数据放回流头部
func 新建回退读取器(连接 net.Conn, 已读数据 []byte) *回退读取器 {
	return &回退读取器{
		剩余数据: bytes.NewReader(已读数据),
		底层连接: 连接,
	}
}

// Read 实现 io.Reader，先返回已读取的数据，再从底层连接读取
func (r *回退读取器) Read(p []byte) (int, error) {
	if r.剩余数据.Len() > 0 {
		return r.剩余数据.Read(p)
	}
	return r.底层连接.Read(p)
}

// Write 实现 net.Conn，委托给底层连接
func (r *回退读取器) Write(p []byte) (int, error) {
	return r.底层连接.Write(p)
}

// Close 实现 net.Conn，关闭底层连接
func (r *回退读取器) Close() error {
	return r.底层连接.Close()
}

// LocalAddr 实现 net.Conn，返回底层连接的本地地址
func (r *回退读取器) LocalAddr() net.Addr {
	return r.底层连接.LocalAddr()
}

// RemoteAddr 实现 net.Conn，返回底层连接的远程地址
func (r *回退读取器) RemoteAddr() net.Addr {
	return r.底层连接.RemoteAddr()
}

// SetDeadline 实现 net.Conn，委托给底层连接
func (r *回退读取器) SetDeadline(t time.Time) error {
	return r.底层连接.SetDeadline(t)
}

// SetReadDeadline 实现 net.Conn，委托给底层连接
func (r *回退读取器) SetReadDeadline(t time.Time) error {
	return r.底层连接.SetReadDeadline(t)
}

// SetWriteDeadline 实现 net.Conn，委托给底层连接
func (r *回退读取器) SetWriteDeadline(t time.Time) error {
	return r.底层连接.SetWriteDeadline(t)
}
