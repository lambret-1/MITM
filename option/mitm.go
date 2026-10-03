package option

// MITMServiceOptions MITM 服务配置选项
// 对应 JSON 中 services[] 内 type=mitm 的配置
type MITMServiceOptions struct {
	// Enabled 是否启用 MITM 解密
	Enabled bool `json:"enabled,omitempty"`

	// CA 根证书与私钥路径配置
	CA MITMCAOptions `json:"ca,omitempty"`

	// Match 需要进行 HTTPS 解密的域名匹配规则
	Match MITMMatchOptions `json:"match,omitempty"`

	// Rewrite HTTP 请求/响应重写配置
	Rewrite MITMRewriteOptions `json:"rewrite,omitempty"`

	// OnError MITM 拦截失败时的处理策略
	// "bypass"：回退到正常路由（默认）
	// "block"：关闭连接
	OnError string `json:"on_error,omitempty"`

	// UpstreamTimeout 上游连接超时（秒），默认 30 秒
	UpstreamTimeout int `json:"upstream_timeout,omitempty"`
}

// MITMCAOptions MITM 根证书配置
// 用于动态签发目标域名的叶子证书
type MITMCAOptions struct {
	// Certificate 根证书文件路径（PEM 格式）
	Certificate string `json:"certificate"`

	// PrivateKey 根证书私钥文件路径（PEM 格式）
	PrivateKey string `json:"private_key"`
}

// MITMMatchOptions MITM 域名匹配规则
// 命中的域名才会进行 TLS 终止与解密
type MITMMatchOptions struct {
	// Domain 精确匹配的域名列表
	Domain []string `json:"domain,omitempty"`

	// DomainSuffix 后缀匹配的域名列表
	DomainSuffix []string `json:"domain_suffix,omitempty"`
}

// MITMRewriteOptions HTTP 重写配置
//
// 参考 README 第 21、22 节，支持请求头/响应头修改和 Body 替换。
// Body 重写会自动处理 gzip/br/zstd 解压与重压缩。
type MITMRewriteOptions struct {
	// Enabled 是否启用 HTTP 重写
	Enabled bool `json:"enabled,omitempty"`

	// MaxBodySize 允许重写的最大 body 字节数（默认 10MB）
	// 超过此大小的响应跳过重写，避免内存溢出（参考 README 第 39 节）
	MaxBodySize int64 `json:"max_body_size,omitempty"`

	// Rules 重写规则列表，按顺序匹配并应用
	Rules []MITMRewriteRule `json:"rules,omitempty"`
}

// MITMRewriteRule 单条 HTTP 重写规则
//
// 匹配条件（域名后缀、路径前缀、方法）全部满足时应用修改。
type MITMRewriteRule struct {
	// DomainSuffix 匹配的域名后缀列表（空表示所有域名）
	DomainSuffix []string `json:"domain_suffix,omitempty"`

	// PathPrefix 匹配的 URL 路径前缀（空表示所有路径）
	PathPrefix string `json:"path_prefix,omitempty"`

	// Method 匹配的 HTTP 方法列表（空表示所有方法）
	Method []string `json:"method,omitempty"`

	// RequestHeader 需要设置/覆盖的请求头
	RequestHeader map[string]string `json:"request_header,omitempty"`

	// RequestHeaderDelete 需要删除的请求头
	RequestHeaderDelete []string `json:"request_header_delete,omitempty"`

	// ResponseHeader 需要设置/覆盖的响应头
	ResponseHeader map[string]string `json:"response_header,omitempty"`

	// ResponseHeaderDelete 需要删除的响应头
	ResponseHeaderDelete []string `json:"response_header_delete,omitempty"`

	// BodyReplace Body 内容替换规则（仅对响应体生效）
	BodyReplace []MITMBodyReplaceRule `json:"body_replace,omitempty"`
}

// MITMBodyReplaceRule Body 内容替换规则
type MITMBodyReplaceRule struct {
	// Find 要查找的字符串
	Find string `json:"find"`

	// Replace 替换为的字符串
	Replace string `json:"replace"`
}
