#!/bin/bash
# sing-box MITM iOS 越狱设备安装脚本
# 使用方法：./install.sh
# 需要在 iOS 越狱设备上以 root 身份运行

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${GREEN}=== sing-box MITM iOS 安装脚本 ===${NC}"

# 检查 root 权限
if [ "$(id -u)" != "0" ]; then
    echo -e "${RED}错误：需要 root 权限运行${NC}"
    exit 1
fi

# 检查架构
ARCH=$(uname -m)
if [ "$ARCH" != "arm64" ]; then
    echo -e "${YELLOW}警告：当前架构 $ARCH，预期 arm64${NC}"
fi

# 安装目录
INSTALL_DIR="/usr/bin"
CONFIG_DIR="/etc/sing-box"
LAUNCHDAEMON_DIR="/Library/LaunchDaemons"

echo -e "${GREEN}[1/5] 安装 sing-box 二进制...${NC}"
cp bin/sing-box "$INSTALL_DIR/sing-box"
chmod 755 "$INSTALL_DIR/sing-box"
chown root:wheel "$INSTALL_DIR/sing-box"
echo "  已安装到 $INSTALL_DIR/sing-box"

echo -e "${GREEN}[2/5] 创建配置目录...${NC}"
mkdir -p "$CONFIG_DIR"
cp config/config.json "$CONFIG_DIR/config.json"
chmod 644 "$CONFIG_DIR/config.json"
chown root:wheel "$CONFIG_DIR/config.json"
echo "  配置文件: $CONFIG_DIR/config.json"

echo -e "${GREEN}[3/5] 生成 MITM CA 证书...${NC}"
if [ ! -f "$CONFIG_DIR/ca.pem" ] || [ ! -f "$CONFIG_DIR/ca.key" ]; then
    "$INSTALL_DIR/sing-box" generate mitm-ca \
        --name "sing-box MITM CA" \
        --validity 3650 \
        --output "$CONFIG_DIR"
    chmod 644 "$CONFIG_DIR/ca.pem"
    chmod 600 "$CONFIG_DIR/ca.key"
    chown root:wheel "$CONFIG_DIR/ca.pem" "$CONFIG_DIR/ca.key"
    echo "  CA 证书已生成"
else
    echo "  CA 证书已存在，跳过"
fi

echo -e "${GREEN}[4/5] 安装 LaunchDaemon...${NC}"
cp launchdaemon/com.sb1.mitm.plist "$LAUNCHDAEMON_DIR/"
chmod 644 "$LAUNCHDAEMON_DIR/com.sb1.mitm.plist"
chown root:wheel "$LAUNCHDAEMON_DIR/com.sb1.mitm.plist"
echo "  LaunchDaemon 已安装"

echo -e "${GREEN}[5/5] 验证配置...${NC}"
if "$INSTALL_DIR/sing-box" check -c "$CONFIG_DIR/config.json"; then
    echo -e "${GREEN}  配置验证通过${NC}"
else
    echo -e "${RED}  配置验证失败，请检查配置文件${NC}"
    exit 1
fi

echo ""
echo -e "${GREEN}=== 安装完成 ===${NC}"
echo ""
echo -e "${YELLOW}下一步操作：${NC}"
echo "1. 将 CA 证书安装到 iOS 系统信任存储："
echo "   - 用文件 App 打开 $CONFIG_DIR/ca.pem"
echo "   - 设置 → 通用 → VPN 与设备管理 → 安装描述文件"
echo "   - 设置 → 通用 → 关于本机 → 证书信任设置 → 启用 sing-box MITM CA"
echo ""
echo "2. 启动服务："
echo "   launchctl load $LAUNCHDAEMON_DIR/com.sb1.mitm.plist"
echo ""
echo "3. 查看日志："
echo "   tail -f /var/log/sing-box.log"
echo ""
echo "4. 停止服务："
echo "   launchctl unload $LAUNCHDAEMON_DIR/com.sb1.mitm.plist"
echo ""
echo -e "${YELLOW}注意：${NC}"
echo "- 默认匹配 example.com / example.org，可修改 config.json 中的 match.domain_suffix"
echo "- 首次运行前请确保已安装 CA 证书并启用信任"
echo "- TUN 模式需要越狱环境的网络权限"
