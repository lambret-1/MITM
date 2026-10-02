package rewrite

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"strings"

	E "github.com/sagernet/sing/common/exceptions"
)

// 体重写器 HTTP Body 重写器
//
// 职责（参考 README 第 22 节）：
//   - 识别 Content-Encoding（identity/gzip/br/zstd）
//   - 解压 → 替换 → 重压缩
//   - 正确更新 Content-Length、Content-Encoding、Transfer-Encoding
//
// 安全限制（参考 README 第 39 节）：
//   - 超过 max_body_size 的 body 跳过重写，避免内存溢出
//   - 禁止无限 io.ReadAll
type 体重写器 struct {
	// 最大体大小 允许重写的最大 body 字节数
	最大体大小 int64
}

// 新建体重写器 创建体重写器
func 新建体重写器(最大体大小 int64) *体重写器 {
	if 最大体大小 <= 0 {
		最大体大小 = 默认最大体大小
	}
	return &体重写器{最大体大小: 最大体大小}
}

// 重写响应体 对 HTTP 响应体应用替换规则
//
// 流程：
//  1. 检查 Content-Length，超过限制则跳过
//  2. 根据 Content-Encoding 解压
//  3. 应用替换规则
//  4. 按原编码重压缩
//  5. 更新 Content-Length 和 Content-Encoding
func (w *体重写器) 重写响应体(resp *http.Response, 替换规则 []体替换规则) error {
	if resp == nil || resp.Body == nil {
		return nil
	}
	if len(替换规则) == 0 {
		return nil
	}

	// 读取 body（带大小限制）
	原始数据, err := io.ReadAll(io.LimitReader(resp.Body, w.最大体大小+1))
	if err != nil {
		return E.Cause(err, "mitm: 读取响应体失败")
	}
	resp.Body.Close()

	// 超过限制则跳过重写，原样返回
	if int64(len(原始数据)) > w.最大体大小 {
		resp.Body = io.NopCloser(bytes.NewReader(原始数据))
		return nil
	}

	// 解压
	编码 := strings.ToLower(resp.Header.Get("Content-Encoding"))
	明文, err := 解压(原始数据, 编码)
	if err != nil {
		return E.Cause(err, "mitm: 解压响应体失败")
	}

	// 应用替换
	修改后 := 明文
	for _, 规则 := range 替换规则 {
		修改后 = bytes.ReplaceAll(修改后, []byte(规则.查找), []byte(规则.替换))
	}

	// 重压缩
	压缩后, err := 压缩(修改后, 编码)
	if err != nil {
		return E.Cause(err, "mitm: 压缩响应体失败")
	}

	// 更新 body 和头
	resp.Body = io.NopCloser(bytes.NewReader(压缩后))
	resp.ContentLength = int64(len(压缩后))
	resp.Header.Set("Content-Length", itoa(int64(len(压缩后))))
	if 编码 == "" {
		resp.Header.Del("Content-Encoding")
	} else {
		resp.Header.Set("Content-Encoding", 编码)
	}
	// body 已修改，删除可能不一致的 Transfer-Encoding
	resp.Header.Del("Transfer-Encoding")

	return nil
}

// 解压 根据编码解压数据
func 解压(数据 []byte, 编码 string) ([]byte, error) {
	switch 编码 {
	case "", "identity":
		return 数据, nil
	case "gzip":
		读取器, err := gzip.NewReader(bytes.NewReader(数据))
		if err != nil {
			return nil, err
		}
		defer 读取器.Close()
		return io.ReadAll(读取器)
	case "br", "zstd":
		// brotli 和 zstd 需要额外依赖，当前阶段跳过
		return 数据, nil
	default:
		return 数据, nil
	}
}

// 压缩 根据编码压缩数据
func 压缩(数据 []byte, 编码 string) ([]byte, error) {
	switch 编码 {
	case "", "identity":
		return 数据, nil
	case "gzip":
		var 缓冲区 bytes.Buffer
		写入器 := gzip.NewWriter(&缓冲区)
		if _, err := 写入器.Write(数据); err != nil {
			return nil, err
		}
		if err := 写入器.Close(); err != nil {
			return nil, err
		}
		return 缓冲区.Bytes(), nil
	case "br", "zstd":
		return 数据, nil
	default:
		return 数据, nil
	}
}

// itoa 简单的整数转字符串（避免引入 strconv 仅用一次）
func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	负 := n < 0
	if 负 {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if 负 {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
