package rewrite

import (
	"net/http"
	"strings"
)

// 匹配器 重写规则匹配器
//
// 负责对 HTTP 请求/响应进行快速匹配，
// 支持域名、路径、方法等维度的组合匹配。
type 匹配器 struct {
	// 规则列表 待匹配的规则列表
	规则列表 []*规则
}

// 新建匹配器 创建匹配器
func 新建匹配器(规则列表 []*规则) *匹配器 {
	return &匹配器{规则列表: 规则列表}
}

// 匹配请求 匹配所有命中请求的规则
func (m *匹配器) 匹配请求(req *http.Request) []*规则 {
	var 命中 []*规则
	for _, 规则 := range m.规则列表 {
		if 规则.匹配请求(req) {
			命中 = append(命中, 规则)
		}
	}
	return 命中
}

// 匹配响应 匹配所有命中响应的规则
func (m *匹配器) 匹配响应(resp *http.Response) []*规则 {
	var 命中 []*规则
	for _, 规则 := range m.规则列表 {
		if 规则.匹配响应(resp) {
			命中 = append(命中, 规则)
		}
	}
	return 命中
}

// 匹配域名 判断主机名是否匹配域名后缀列表
func 匹配域名(主机名 string, 后缀列表 []string) bool {
	主机名 = strings.ToLower(strings.TrimSuffix(主机名, "."))
	for _, 后缀 := range 后缀列表 {
		后缀 = strings.ToLower(strings.TrimSuffix(后缀, "."))
		if 主机名 == 后缀 || strings.HasSuffix(主机名, "."+后缀) {
			return true
		}
	}
	return false
}

// 匹配路径 判断路径是否匹配前缀
func 匹配路径(路径 string, 前缀 string) bool {
	if 前缀 == "" {
		return true
	}
	return strings.HasPrefix(路径, 前缀)
}

// 匹配方法 判断方法是否在允许列表中
func 匹配方法(方法 string, 允许列表 []string) bool {
	if len(允许列表) == 0 {
		return true
	}
	for _, 允许 := range 允许列表 {
		if strings.EqualFold(允许, 方法) {
			return true
		}
	}
	return false
}
