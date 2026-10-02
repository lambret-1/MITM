package mitm

import (
	"io"
	"net"
	"net/http"

	E "github.com/sagernet/sing/common/exceptions"
)

// WebSocket处理器 WebSocket 协议处理器
//
// 职责（参考 README 第 20 节）：
//   - 识别 HTTP Upgrade: websocket 请求
//   - 完成 101 Switching Protocols 握手
//   - 握手后转为原始双向流转发
//   - 处理 half close、context cancellation、close frames、backpressure、deadlines
//
// 握手后不能继续把 WebSocket frame 当普通 HTTP body 处理。
type WebSocket处理器 struct {
	服务 *Service
}

// 新建WebSocket处理器 创建 WebSocket 处理器
func 新建WebSocket处理器(服务 *Service) *WebSocket处理器 {
	return &WebSocket处理器{服务: 服务}
}

// 是否WebSocketUpgrade 判断 HTTP 请求是否为 WebSocket 升级请求
func 是否WebSocketUpgrade(req *http.Request) bool {
	if req == nil {
		return false
	}
	连接头 := req.Header.Get("Connection")
	升级头 := req.Header.Get("Upgrade")
	return 连接头 == "Upgrade" && 升级头 == "websocket"
}

// 处理握手 完成 WebSocket 握手并返回原始双向流
//
// 握手成功后，客户端连接和上游连接都进入 raw bidirectional stream 状态，
// 最基本的 relay 是双向 io.Copy。
func (h *WebSocket处理器) 处理握手(客户端连接 net.Conn, req *http.Request, 上游连接 net.Conn) error {
	// 后续阶段实现：
	// 1. 向上游发送 Upgrade 请求
	// 2. 接收上游 101 响应
	// 3. 向客户端发送 101 响应
	// 4. 双向 io.Copy（带 context 取消和 backpressure 处理）

	_ = req
	_ = 客户端连接
	_ = 上游连接
	return E.New("mitm: WebSocket 处理尚未实现（待 Phase 4）")
}

// 双向转发 在客户端和上游之间双向转发数据
//
// 生产版本需要处理：
//   - half close（一方关闭写后另一方继续读）
//   - context cancellation（及时中断）
//   - close frames（WebSocket 关闭握手）
//   - backpressure（流量控制）
//   - deadlines（超时断开）
func 双向转发(客户端 net.Conn, 上游 net.Conn) error {
	错误通道 := make(chan error, 2)

	go func() {
		_, err := io.Copy(上游, 客户端)
		错误通道 <- err
	}()
	go func() {
		_, err := io.Copy(客户端, 上游)
		错误通道 <- err
	}()

	// 等待任意一方结束
	err := <-错误通道
	if err != nil && err != io.EOF {
		return E.Cause(err, "mitm: WebSocket 双向转发失败")
	}
	return nil
}
