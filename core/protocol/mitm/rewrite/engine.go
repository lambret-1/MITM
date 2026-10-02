// Package rewrite MITM HTTP 重写引擎
//
// 职责（参考 README 第 21 节）：
//   - 根据规则匹配 HTTP 请求/响应
//   - 修改请求头、响应头
//   - 修改请求体、响应体（含解压/重压缩）
//   - 正确更新 Content-Length、Content-Encoding、Transfer-Encoding
package rewrite

import (
	"net/http"

	"github.com/sagernet/sing-box/option"
)

// 引擎 HTTP 重写引擎
//
// 持有重写规则列表，对每个请求/响应依次匹配并应用。
type 引擎 struct {
	// 规则列表 重写规则列表，按顺序匹配
	规则列表 []*规则
	// 启用 是否启用重写
	启用 bool
	// 最大体大小 允许重写的最大 body 字节数（参考 README 第 39 节）
	最大体大小 int64
}

// 默认最大体大小 默认 10MB
const 默认最大体大小 = 10 * 1024 * 1024

// 新建引擎 根据配置创建重写引擎
func 新建引擎(配置 option.MITMRewriteOptions) *引擎 {
	return &引擎{
		启用:       配置.Enabled,
		最大体大小: 默认最大体大小,
	}
}

// 重写请求 对 HTTP 请求应用重写规则
//
// 流程：
//  1. 遍历规则列表，匹配请求
//  2. 应用请求头修改
//  3. 如需 body 重写，读取并修改 body
//  4. 更新 Content-Length 等头
func (e *引擎) 重写请求(req *http.Request) error {
	if !e.启用 || req == nil {
		return nil
	}
	// 后续阶段实现
	for _, 规则 := range e.规则列表 {
		if 规则.匹配请求(req) {
			规则.应用请求头(req)
		}
	}
	return nil
}

// 重写响应 对 HTTP 响应应用重写规则
//
// 流程：
//  1. 遍历规则列表，匹配响应
//  2. 应用响应头修改
//  3. 如需 body 重写，解压→修改→重压缩
//  4. 更新 Content-Length、Content-Encoding
func (e *引擎) 重写响应(resp *http.Response) error {
	if !e.启用 || resp == nil {
		return nil
	}
	// 后续阶段实现
	for _, 规则 := range e.规则列表 {
		if 规则.匹配响应(resp) {
			规则.应用响应头(resp)
		}
	}
	return nil
}

// 添加规则 向引擎添加重写规则
func (e *引擎) 添加规则(规则 *规则) {
	e.规则列表 = append(e.规则列表, 规则)
}
