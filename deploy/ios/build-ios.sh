#!/bin/bash
# sing-box MITM iOS 交叉编译脚本
# 在 Linux 上交叉编译 darwin/arm64 二进制，可在 iOS 越狱设备上运行
#
# 使用方法：./build-ios.sh
# 依赖：Go 1.25+

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
OUTPUT_DIR="$SCRIPT_DIR/bin"

echo "=== sing-box MITM iOS 交叉编译 ==="
echo "项目目录: $PROJECT_DIR"
echo "输出目录: $OUTPUT_DIR"

# 检查 Go
if ! command -v go &> /dev/null; then
    echo "错误：未找到 go 命令"
    exit 1
fi

GO_VERSION=$(go version | awk '{print $3}')
echo "Go 版本: $GO_VERSION"

# 创建输出目录
mkdir -p "$OUTPUT_DIR"

# 交叉编译 darwin/arm64
echo ""
echo "编译 darwin/arm64..."
cd "$PROJECT_DIR"
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 \
    go build \
    -o "$OUTPUT_DIR/sing-box" \
    -trimpath \
    -ldflags="-s -w" \
    ./cmd/sing-box

# 验证二进制
echo ""
echo "编译完成！"
ls -lh "$OUTPUT_DIR/sing-box"
file "$OUTPUT_DIR/sing-box"

echo ""
echo "=== 构建完成 ==="
echo "二进制路径: $OUTPUT_DIR/sing-box"
echo ""
echo "部署到 iOS 设备："
echo "  scp $OUTPUT_DIR/sing-box root@<设备IP>:/usr/bin/sing-box"
echo "  ssh root@<设备IP> 'chmod 755 /usr/bin/sing-box'"
echo ""
echo "注意："
echo "  1. 此二进制为 darwin/arm64，可在 iOS 越狱设备上运行"
echo "  2. 如遇代码签名问题，使用 ldid -S /usr/bin/sing-box 进行伪签名"
echo "  3. TUN 功能需要越狱环境的 root 权限"
