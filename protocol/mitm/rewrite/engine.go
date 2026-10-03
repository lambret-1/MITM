// Package rewrite MITM HTTP 重写引擎
//
// 职责（参考 README 第 21、22 节）：
//   - 根据规则匹配 HTTP 请求/响应
//   - 修改请求头、响应头
//   - 修改响应体（含 gzip/br/zstd 解压/重压缩）
//   - 正确更新 Content-Length、Content-Encoding、Transfer-Encoding
package rewrite

import (
	"net/http"

	"github.com/sagernet/sing-box/option"
)

// Engine 引擎：HTTP 重写引擎
//
// 持有重写规则列表，对每个请求/响应依次匹配并应用。
type Engine struct {
	// 规则列表 重写规则列表，按顺序匹配
	规则列表 []*规则
	// 启用 是否启用重写
	启用 bool
	// 最大体大小 允许重写的最大 body 字节数（参考 README 第 39 节）
	最大体大小 int64
	// 体重写器 响应体重写器
	体重写器 *体重写器
}

// 默认最大体大小 默认 10MB
const 默认最大体大小 = 10 * 1024 * 1024

// NewEngine 新建引擎：根据配置创建重写引擎
func NewEngine(配置 option.MITMRewriteOptions) *Engine {
	最大体大小 := 配置.MaxBodySize
	if 最大体大小 <= 0 {
		最大体大小 = 默认最大体大小
	}

	e := &Engine{
		启用:       配置.Enabled,
		最大体大小: 最大体大小,
		体重写器:   新建体重写器(最大体大小),
	}

	// 从配置加载规则
	for _, 规则配置 := range 配置.Rules {
		规则 := &规则{
			域名后缀:   规则配置.DomainSuffix,
			路径前缀:   规则配置.PathPrefix,
			方法:       规则配置.Method,
			请求头设置: 规则配置.RequestHeader,
			请求头删除: 规则配置.RequestHeaderDelete,
			响应头设置: 规则配置.ResponseHeader,
			响应头删除: 规则配置.ResponseHeaderDelete,
		}
		for _, 替换 := range 规则配置.BodyReplace {
			规则.体替换列表 = append(规则.体替换列表, 体替换规则{
				查找: 替换.Find,
				替换: 替换.Replace,
			})
		}
		e.规则列表 = append(e.规则列表, 规则)
	}

	return e
}

// RewriteRequest 重写请求：对 HTTP 请求应用重写规则
//
// 流程：
//  1. 遍历规则列表，匹配请求
//  2. 应用请求头修改（设置/删除）
func (e *Engine) RewriteRequest(req *http.Request) error {
	if !e.启用 || req == nil {
		return nil
	}
	for _, 规则 := range e.规则列表 {
		if 规则.匹配请求(req) {
			规则.应用请求头(req)
		}
	}
	return nil
}

// RewriteResponse 重写响应：对 HTTP 响应应用重写规则
//
// 流程：
//  1. 遍历规则列表，匹配响应（基于关联的请求）
//  2. 应用响应头修改（设置/删除）
//  3. 收集所有命中规则的 Body 替换规则
//  4. 调用体重写器：解压 → 替换 → 重压缩 → 更新头
func (e *Engine) RewriteResponse(resp *http.Response) error {
	if !e.启用 || resp == nil {
		return nil
	}

	var 所有体替换 []体替换规则
	for _, 规则 := range e.规则列表 {
		if 规则.匹配响应(resp) {
			规则.应用响应头(resp)
			所有体替换 = append(所有体替换, 规则.体替换列表...)
		}
	}

	// 应用 Body 重写
	if len(所有体替换) > 0 && e.体重写器 != nil {
		if err := e.体重写器.重写响应体(resp, 所有体替换); err != nil {
			return err
		}
	}

	return nil
}

// 添加规则 向引擎添加重写规则
func (e *Engine) 添加规则(规则 *规则) {
	e.规则列表 = append(e.规则列表, 规则)
}

// IsEnabled 是否启用：返回重写引擎是否启用
func (e *Engine) IsEnabled() bool {
	return e.启用
}
