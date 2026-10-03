package mitm

import (
	"crypto/tls"
	"container/list"
	"sync"
)

// 默认缓存容量 叶子证书缓存的最大条目数
// 参考 README 第 40 节，长期运行环境建议 4096
const 默认缓存容量 = 4096

// 证书缓存 基于 LRU 的叶子证书缓存
// 避免为同一域名重复签发证书，降低 CPU 开销。
//
// 线程安全，支持并发读写。
type 证书缓存 struct {
	锁       sync.Mutex
	容量     int
	缓存     map[string]*list.Element
	访问列表 *list.List
}

// 缓存条目 缓存中的单个条目
type 缓存条目 struct {
	域名    string
	证书    *tls.Certificate
}

// 新证书缓存 创建指定容量的证书缓存
func 新证书缓存(容量 int) *证书缓存 {
	if 容量 <= 0 {
		容量 = 默认缓存容量
	}
	return &证书缓存{
		容量:     容量,
		缓存:     make(map[string]*list.Element),
		访问列表: list.New(),
	}
}

// 获取 从缓存中获取域名对应的叶子证书
// 返回证书和是否命中。命中时将条目移到访问列表头部（最近使用）。
func (c *证书缓存) 获取(域名 string) (*tls.Certificate, bool) {
	c.锁.Lock()
	defer c.锁.Unlock()
	if 元素, 存在 := c.缓存[域名]; 存在 {
		c.访问列表.MoveToFront(元素)
		return 元素.Value.(*缓存条目).证书, true
	}
	return nil, false
}

// 存入 将域名对应的叶子证书存入缓存
// 若缓存已满，淘汰最久未使用的条目。
func (c *证书缓存) 存入(域名 string, 证书 *tls.Certificate) {
	c.锁.Lock()
	defer c.锁.Unlock()

	// 已存在则更新并移到头部
	if 元素, 存在 := c.缓存[域名]; 存在 {
		c.访问列表.MoveToFront(元素)
		元素.Value.(*缓存条目).证书 = 证书
		return
	}

	// 新增条目
	条目 := &缓存条目{域名: 域名, 证书: 证书}
	元素 := c.访问列表.PushFront(条目)
	c.缓存[域名] = 元素

	// 超出容量则淘汰最久未使用
	if c.访问列表.Len() > c.容量 {
		淘汰元素 := c.访问列表.Back()
		if 淘汰元素 != nil {
			淘汰条目 := 淘汰元素.Value.(*缓存条目)
			delete(c.缓存, 淘汰条目.域名)
			c.访问列表.Remove(淘汰元素)
		}
	}
}

// 清空 清空所有缓存条目
func (c *证书缓存) 清空() {
	c.锁.Lock()
	defer c.锁.Unlock()
	c.缓存 = make(map[string]*list.Element)
	c.访问列表.Init()
}

// 数量 返回当前缓存条目数
func (c *证书缓存) 数量() int {
	c.锁.Lock()
	defer c.锁.Unlock()
	return c.访问列表.Len()
}
