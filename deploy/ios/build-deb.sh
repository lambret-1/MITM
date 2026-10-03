#!/bin/bash
# sing-box MITM iOS .deb 安装包构建脚本
# 适用于多巴胺(Dopamine)、palera1n、checkra1n 等越狱设备
# 无需电脑操作：deb 包可通过 Filza/Sileo/Cydia 直接安装
# 兼容 Rootless（/var/jb/ 前缀）和 Rootful 环境

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
DEB_ROOT="$SCRIPT_DIR/deb"
OUTPUT_DIR="$SCRIPT_DIR/output"
PACKAGE_NAME="com.sb1.mitm"
VERSION="1.11.0-1"
ARCH="iphoneos-arm64"
DEB_FILE="${PACKAGE_NAME}_${VERSION}_${ARCH}.deb"

echo "========================================"
echo "  sing-box MITM iOS .deb 构建"
echo "========================================"
echo "项目目录: $PROJECT_DIR"
echo "输出目录: $OUTPUT_DIR"
echo "安装前缀: /var/jb/（多巴胺 Rootless 兼容）"
echo ""

# 检查 Go
if ! command -v go &> /dev/null; then
    echo "错误：未找到 go 命令"
    exit 1
fi

# 创建输出目录
mkdir -p "$OUTPUT_DIR"

# 1. 交叉编译 darwin/arm64 二进制
echo "[1/4] 交叉编译 darwin/arm64 二进制..."
cd "$PROJECT_DIR"
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 \
    go build \
    -o "$DEB_ROOT/var/jb/usr/bin/sing-box" \
    -trimpath \
    -ldflags="-s -w" \
    ./cmd/sing-box
chmod 755 "$DEB_ROOT/var/jb/usr/bin/sing-box"
ls -lh "$DEB_ROOT/var/jb/usr/bin/sing-box"

# 1.5 生成 CA 证书（如果不存在，用 openssl 生成 ECDSA P-256 CA）
echo "[1.5/4] 检查 CA 证书..."
CA_CERT="$DEB_ROOT/var/jb/etc/sing-box/ca.pem"
CA_KEY="$DEB_ROOT/var/jb/etc/sing-box/ca.key"
if [ ! -f "$CA_CERT" ] || [ ! -f "$CA_KEY" ]; then
    echo "  CA 证书不存在，用 openssl 生成..."
    if command -v openssl &> /dev/null; then
        openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 \
            -keyout "$CA_KEY" -out "$CA_CERT" -days 3650 -nodes \
            -subj "/CN=SB1 MITM CA" \
            -addext "basicConstraints=critical,CA:TRUE" \
            -addext "keyUsage=critical,keyCertSign,cRLSign,digitalSignature" 2>/dev/null
        chmod 644 "$CA_CERT"
        chmod 600 "$CA_KEY"
        echo "  CA 证书已生成: $CA_CERT"
    else
        echo "  警告：未找到 openssl，CA 证书缺失（CI 流水线会自动预生成）"
    fi
else
    echo "  CA 证书已存在，跳过"
fi

# 2. 设置权限
echo "[2/4] 设置文件权限..."
chmod 755 "$DEB_ROOT/DEBIAN/preinst" "$DEB_ROOT/DEBIAN/postinst" "$DEB_ROOT/DEBIAN/prerm" "$DEB_ROOT/DEBIAN/postrm"
chmod 644 "$DEB_ROOT/DEBIAN/control"
chmod 644 "$DEB_ROOT/var/jb/etc/sing-box/config.json"
chmod 644 "$DEB_ROOT/var/jb/etc/sing-box/ca.pem" 2>/dev/null || true
chmod 600 "$DEB_ROOT/var/jb/etc/sing-box/ca.key" 2>/dev/null || true
chmod 644 "$DEB_ROOT/var/jb/Library/LaunchDaemons/com.sb1.mitm.plist"

# 3. 构建 .deb 包
echo "[3/4] 构建 .deb 包..."
cd "$SCRIPT_DIR"

if command -v dpkg-deb &> /dev/null; then
    # 使用 dpkg-deb，强制 gzip 压缩（iOS 越狱环境旧版 dpkg 不支持 zstd）
    dpkg-deb --build -Zgzip --root-owner-group "$DEB_ROOT" "$OUTPUT_DIR/$DEB_FILE"
else
    # 手动构建 .deb（ar + tar）
    echo "  使用手动方式构建（ar + tar.gz）..."

    # 创建临时目录
    TMP_DIR=$(mktemp -d)

    # debian-binary
    echo "2.0" > "$TMP_DIR/debian-binary"

    # control.tar.gz
    tar -czf "$TMP_DIR/control.tar.gz" -C "$DEB_ROOT/DEBIAN" .

    # data.tar.gz
    tar --owner=0 --group=0 -czf "$TMP_DIR/data.tar.gz" \
        -C "$DEB_ROOT" \
        ./var

    # 组合为 .deb（ar 格式）
    ar r "$OUTPUT_DIR/$DEB_FILE" \
        "$TMP_DIR/debian-binary" \
        "$TMP_DIR/control.tar.gz" \
        "$TMP_DIR/data.tar.gz"

    rm -rf "$TMP_DIR"
fi

# 4. 验证
echo "[4/4] 验证 .deb 包..."
ls -lh "$OUTPUT_DIR/$DEB_FILE"

if command -v dpkg-deb &> /dev/null; then
    dpkg-deb --info "$OUTPUT_DIR/$DEB_FILE"
    dpkg-deb --contents "$OUTPUT_DIR/$DEB_FILE"
fi

echo ""
echo "========================================"
echo "  构建完成！"
echo "========================================"
echo "deb 包: $OUTPUT_DIR/$DEB_FILE"
echo ""
echo "安装方式（无需电脑）："
echo "  1. 将 .deb 文件传输到 iOS 设备（AirDrop/邮件/网盘）"
echo "  2. 用 Filza 文件管理器打开，点击安装"
echo "  3. 或在终端执行: dpkg -i $DEB_FILE"
echo "  4. 或通过 Sileo/Cydia 添加源安装"
echo ""
echo "安装后请按提示完成 CA 证书信任设置"
echo "========================================"
