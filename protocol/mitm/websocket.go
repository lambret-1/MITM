package mitm

import (
	"net/http"
	"strings"
)

// WebSocket处理器 WebSocket 协议处理器
//
// 职责（参考 README 第 20 节）：
//   - 识别 HTTP Upgrade: websocket 请求
//   - 握手完成后转为原始双向流转发
//   - 处理 half close、context cancellation、close frames、backpressure、deadlines
//
// 实际握手和转发逻辑在 HTTP1处理器.处理WebSocket 中实现。
type WebSocket处理器 struct {
	服务 *Service
}

// 新建WebSocket处理器 创建 WebSocket 处理器
func 新建WebSocket处理器(服务 *Service) *WebSocket处理器 {
	return &WebSocket处理器{服务: 服务}
}

// 是否WebSocketUpgrade 判断 HTTP 请求是否为 WebSocket 升级请求
//
// 典型请求头：
//
//	Connection: Upgrade
//	Upgrade: websocket
func 是否WebSocketUpgrade(req *http.Request) bool {
	if req == nil {
		return false
	}
	连接头 := strings.ToLower(req.Header.Get("Connection"))
	升级头 := strings.ToLower(req.Header.Get("Upgrade"))
	return strings.Contains(连接头, "upgrade") && 升级头 == "websocket"
}
