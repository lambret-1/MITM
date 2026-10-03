package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/sagernet/sing-box/log"

	"github.com/spf13/cobra"
)

// mitm-ca 命令参数
var (
	// flagGenerateMITMCAValidity 证书有效期（天）
	flagGenerateMITMCAValidity int
	// flagGenerateMITMCAName 证书名称（CN）
	flagGenerateMITMCAName string
	// flagGenerateMITMCAOutput 输出目录
	flagGenerateMITMCAOutput string
)

// commandGenerateMITMCA sing-box generate mitm-ca 命令
//
// 生成 MITM 根证书和私钥，用于动态签发目标域名的叶子证书。
// 生成的 CA 证书需要安装到 iOS 系统信任存储中。
var commandGenerateMITMCA = &cobra.Command{
	Use:   "mitm-ca",
	Short: "Generate MITM root CA certificate and private key",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		err := generateMITMCA()
		if err != nil {
			log.Fatal(err)
		}
	},
}

func init() {
	commandGenerateMITMCA.Flags().IntVarP(&flagGenerateMITMCAValidity, "validity", "v", 3650, "Certificate validity in days")
	commandGenerateMITMCA.Flags().StringVarP(&flagGenerateMITMCAName, "name", "n", "sing-box MITM CA", "Certificate common name")
	commandGenerateMITMCA.Flags().StringVarP(&flagGenerateMITMCAOutput, "output", "o", ".", "Output directory")
	commandGenerate.AddCommand(commandGenerateMITMCA)
}

// generateMITMCA 生成 MITM 根证书和私钥
//
// 使用 ECDSA P-256 算法，证书属性：
//   - IsCA: true
//   - KeyUsage: CertSign
//   - BasicConstraintsValid: true
//   - 序列号随机
//   - 有效期从当前时间起 validity 天
func generateMITMCA() error {
	// 生成 ECDSA P-256 私钥
	私钥, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}

	// 生成随机序列号
	序列号, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}

	// 构造证书模板
	模板 := x509.Certificate{
		SerialNumber: 序列号,
		Subject: pkix.Name{
			CommonName:   flagGenerateMITMCAName,
			Organization: []string{"sing-box"},
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().AddDate(0, 0, flagGenerateMITMCAValidity),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}

	// 自签名证书
	证书DER, err := x509.CreateCertificate(rand.Reader, &模板, &模板, &私钥.PublicKey, 私钥)
	if err != nil {
		return err
	}

	// 确保输出目录存在
	if err := os.MkdirAll(flagGenerateMITMCAOutput, 0755); err != nil {
		return err
	}

	// 写入证书 PEM
	证书路径 := filepath.Join(flagGenerateMITMCAOutput, "ca.pem")
	证书文件, err := os.Create(证书路径)
	if err != nil {
		return err
	}
	pem.Encode(证书文件, &pem.Block{Type: "CERTIFICATE", Bytes: 证书DER})
	证书文件.Close()
	os.Chmod(证书路径, 0644)

	// 写入私钥 PEM
	私钥DER, err := x509.MarshalECPrivateKey(私钥)
	if err != nil {
		return err
	}
	私钥路径 := filepath.Join(flagGenerateMITMCAOutput, "ca.key")
	私钥文件, err := os.Create(私钥路径)
	if err != nil {
		return err
	}
	pem.Encode(私钥文件, &pem.Block{Type: "EC PRIVATE KEY", Bytes: 私钥DER})
	私钥文件.Close()
	os.Chmod(私钥路径, 0600)

	log.Info("mitm: CA 证书已生成: ", 证书路径)
	log.Info("mitm: CA 私钥已生成: ", 私钥路径)
	log.Info("mitm: 证书有效期: ", flagGenerateMITMCAValidity, " 天")
	log.Info("mitm: 请将 ca.pem 安装到 iOS 系统信任存储")
	return nil
}
