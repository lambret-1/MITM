package mitm

import (
	"strings"

	"github.com/sagernet/sing-box/option"
)

// 域名匹配器 MITM 域名匹配规则
// 命中的域名才会进行 TLS 终止与解密。
//
// 支持两种匹配方式：
//   - domain：精确匹配
//   - domain_suffix：后缀匹配（example.com 匹配 api.example.com）
type 域名匹配器 struct {
	// 精确匹配集合
	精确集合 map[string]struct{}
	// 后缀匹配列表
	后缀列表 []string
}

// 新域名匹配器 根据配置创建域名匹配器
func 新域名匹配器(配置 option.MITMMatchOptions) *域名匹配器 {
	m := &域名匹配器{
		精确集合: make(map[string]struct{}),
	}
	for _, 域名 := range 配置.Domain {
		m.精确集合[域名] = struct{}{}
	}
	m.后缀列表 = append(m.后缀列表, 配置.DomainSuffix...)
	return m
}

// 匹配 判断给定域名是否命中 MITM 规则
// 优先精确匹配，未命中再尝试后缀匹配。
func (m *域名匹配器) 匹配(域名 string) bool {
	if m == nil {
		return false
	}
	域名 = strings.ToLower(strings.TrimSuffix(域名, "."))

	// 精确匹配
	if _, 命中 := m.精确集合[域名]; 命中 {
		return true
	}

	// 后缀匹配
	for _, 后缀 := range m.后缀列表 {
		后缀 = strings.ToLower(strings.TrimSuffix(后缀, "."))
		if 域名 == 后缀 {
			return true
		}
		if strings.HasSuffix(域名, "."+后缀) {
			return true
		}
	}
	return false
}

// 是否为空 判断匹配器是否没有任何规则
func (m *域名匹配器) 是否为空() bool {
	if m == nil {
		return true
	}
	return len(m.精确集合) == 0 && len(m.后缀列表) == 0
}
