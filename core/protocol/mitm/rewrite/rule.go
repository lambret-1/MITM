package rewrite

import (
	"net/http"
	"strings"
)

// 规则 单条 HTTP 重写规则
//
// 参考 README 第 21 节，规则包含：
//   - 匹配条件：域名后缀、路径前缀、方法
//   - 请求头修改
//   - 响应头修改
//   - Body 替换规则
type 规则 struct {
	// 域名后缀 匹配的域名后缀列表
	域名后缀 []string
	// 路径前缀 匹配的 URL 路径前缀
	路径前缀 string
	// 方法 匹配的 HTTP 方法列表（空表示所有方法）
	方法 []string

	// 请求头设置 需要设置/覆盖的请求头
	请求头设置 map[string]string
	// 请求头删除 需要删除的请求头
	请求头删除 []string

	// 响应头设置 需要设置/覆盖的响应头
	响应头设置 map[string]string
	// 响应头删除 需要删除的响应头
	响应头删除 []string

	// 体替换列表 Body 替换规则
	体替换列表 []体替换规则
}

// 体替换规则 Body 内容替换规则
type 体替换规则 struct {
	// 查找 要查找的字符串
	查找 string
	// 替换 替换为的字符串
	替换 string
}

// 匹配请求 判断规则是否匹配给定请求
func (r *规则) 匹配请求(req *http.Request) bool {
	if req == nil {
		return false
	}
	// 域名匹配
	if len(r.域名后缀) > 0 {
		主机名 := req.URL.Hostname()
		匹配 := false
		for _, 后缀 := range r.域名后缀 {
			if 主机名 == 后缀 || strings.HasSuffix(主机名, "."+后缀) {
				匹配 = true
				break
			}
		}
		if !匹配 {
			return false
		}
	}
	// 路径前缀匹配
	if r.路径前缀 != "" && !strings.HasPrefix(req.URL.Path, r.路径前缀) {
		return false
	}
	// 方法匹配
	if len(r.方法) > 0 {
		匹配 := false
		for _, 方法 := range r.方法 {
			if strings.EqualFold(方法, req.Method) {
				匹配 = true
				break
			}
		}
		if !匹配 {
			return false
		}
	}
	return true
}

// 匹配响应 判断规则是否匹配给定响应
// 响应匹配基于请求信息，因此需要传入原始请求
func (r *规则) 匹配响应(resp *http.Response) bool {
	if resp == nil || resp.Request == nil {
		return false
	}
	return r.匹配请求(resp.Request)
}

// 应用请求头 对请求应用头修改
func (r *规则) 应用请求头(req *http.Request) {
	for 键, 值 := range r.请求头设置 {
		req.Header.Set(键, 值)
	}
	for _, 键 := range r.请求头删除 {
		req.Header.Del(键)
	}
}

// 应用响应头 对响应应用头修改
func (r *规则) 应用响应头(resp *http.Response) {
	for 键, 值 := range r.响应头设置 {
		resp.Header.Set(键, 值)
	}
	for _, 键 := range r.响应头删除 {
		resp.Header.Del(键)
	}
}
