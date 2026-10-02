package option

// MITMServiceOptions MITM 服务配置选项
// 对应 JSON 中 services[] 内 type=mitm 的 options 字段
type MITMServiceOptions struct {
	// Enabled 是否启用 MITM 解密
	Enabled bool `json:"enabled,omitempty"`

	// CA 根证书与私钥路径配置
	CA MITMCAOptions `json:"ca,omitempty"`

	// Match 需要进行 HTTPS 解密的域名匹配规则
	Match MITMMatchOptions `json:"match,omitempty"`

	// Rewrite HTTP 请求/响应重写配置
	Rewrite MITMRewriteOptions `json:"rewrite,omitempty"`
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
type MITMRewriteOptions struct {
	// Enabled 是否启用 HTTP 重写
	Enabled bool `json:"enabled,omitempty"`
}
