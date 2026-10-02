package mitm

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"time"

	E "github.com/sagernet/sing/common/exceptions"
)

// 叶子证书有效期 默认 30 天
const 叶子证书有效期 = 30 * 24 * time.Hour

// 签发叶子证书 使用根证书为指定域名签发叶子证书
//
// 证书属性（参考 README 第 14 节）：
//   - Subject.CN = 域名
//   - DNSNames = [域名]
//   - ExtKeyUsage = ServerAuth
//   - KeyUsage = DigitalSignature
//   - Issuer = 根证书
//
// 签发后自动存入 LRU 缓存，后续同域名直接复用。
func (s *Service) 签发叶子证书(域名 string) (*tls.Certificate, error) {
	if s.根证书 == nil {
		return nil, E.New("mitm: 根证书未加载，无法签发叶子证书")
	}
	if 域名 == "" {
		return nil, E.New("mitm: 签发叶子证书时域名为空")
	}

	// 先查缓存
	if 证书, 命中 := s.叶子缓存.获取(域名); 命中 {
		return 证书, nil
	}

	s.证书锁.Lock()
	defer s.证书锁.Unlock()

	// 双重检查，避免并发重复签发
	if 证书, 命中 := s.叶子缓存.获取(域名); 命中 {
		return 证书, nil
	}

	// 生成叶子证书 ECDSA P256 密钥对
	叶子私钥, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, 包装错误(err, "生成叶子证书密钥失败")
	}

	// 生成随机序列号
	序列号, err := 随机序列号()
	if err != nil {
		return nil, 包装错误(err, "生成证书序列号失败")
	}

	// 构造证书模板
	模板 := &x509.Certificate{
		SerialNumber: 序列号,
		Subject: pkix.Name{
			CommonName: 域名,
		},
		DNSNames:    []string{域名},
		NotBefore:   time.Now().Add(-time.Hour),
		NotAfter:    time.Now().Add(叶子证书有效期),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	// 用根证书签发
	证书DER, err := x509.CreateCertificate(rand.Reader, 模板, s.根证书.证书实体, &叶子私钥.PublicKey, s.根证书.证书对.PrivateKey)
	if err != nil {
		return nil, 包装错误(err, "签发叶子证书失败: "+域名)
	}

	// 组装 tls.Certificate
	叶子证书对 := &tls.Certificate{
		Certificate: [][]byte{证书DER},
		PrivateKey:  叶子私钥,
	}

	// 存入缓存
	s.叶子缓存.存入(域名, 叶子证书对)
	return 叶子证书对, nil
}

// 随机序列号 生成 128 位随机证书序列号
func 随机序列号() (*big.Int, error) {
	序列号限制 := new(big.Int).Lsh(big.NewInt(1), 128)
	return rand.Int(rand.Reader, 序列号限制)
}
