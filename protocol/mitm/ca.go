package mitm

import (
	"crypto/tls"
	"crypto/x509"
	"os"

	"github.com/sagernet/sing-box/option"
	E "github.com/sagernet/sing/common/exceptions"
)

// 预定义错误
var (
	// 错误缺少证书路径 未配置根证书文件路径
	错误缺少证书路径 = E.New("mitm: 缺少根证书文件路径 (ca.certificate)")
	// 错误缺少私钥路径 未配置根证书私钥文件路径
	错误缺少私钥路径 = E.New("mitm: 缺少根证书私钥文件路径 (ca.private_key)")
)

// 包装错误 为错误添加 mitm 前缀
func 包装错误(err error, 信息 string) error {
	return E.Cause(err, "mitm: ", 信息)
}

// 根证书实例 解析后的根证书，包含证书对和 x509 实体
type 根证书实例 struct {
	// 证书对 tls.Certificate，含私钥，用于签发叶子证书
	证书对 tls.Certificate
	// 证书实体 x509.Certificate，用于设置 Issuer 等字段
	证书实体 *x509.Certificate
}

// 加载根证书 读取 PEM 格式的证书与私钥，验证其为合法 CA 证书
//
// 校验项：
//  1. 文件可读
//  2. 证书与私钥能配对（tls.X509KeyPair）
//  3. 证书实体可解析
//  4. IsCA == true
//  5. KeyUsage 包含 KeyUsageCertSign
func 加载根证书(配置 option.MITMCAOptions) (*根证书实例, error) {
	证书PEM, err := os.ReadFile(配置.Certificate)
	if err != nil {
		return nil, E.Cause(err, "读取根证书文件失败: ", 配置.Certificate)
	}

	私钥PEM, err := os.ReadFile(配置.PrivateKey)
	if err != nil {
		return nil, E.Cause(err, "读取根证书私钥文件失败: ", 配置.PrivateKey)
	}

	// 加载证书+私钥对
	证书对, err := tls.X509KeyPair(证书PEM, 私钥PEM)
	if err != nil {
		return nil, E.Cause(err, "解析根证书与私钥失败")
	}

	// 解析证书实体以验证 CA 属性
	if len(证书对.Certificate) == 0 {
		return nil, E.New("根证书中不包含任何证书")
	}
	证书实体, err := x509.ParseCertificate(证书对.Certificate[0])
	if err != nil {
		return nil, E.Cause(err, "解析根证书实体失败")
	}
	if !证书实体.IsCA {
		return nil, E.New("提供的证书不是 CA 证书 (IsCA=false)，无法用于签发叶子证书")
	}
	if (证书实体.KeyUsage & x509.KeyUsageCertSign) == 0 {
		return nil, E.New("提供的 CA 证书缺少 KeyUsageCertSign 权限，无法签发证书")
	}

	return &根证书实例{
		证书对:   证书对,
		证书实体: 证书实体,
	}, nil
}
