# iOS 越狱部署指南

本文档说明如何在 iOS 越狱设备上部署 sing-box MITM 模块。

## 1. 生成 MITM 根证书

在电脑上生成 CA 证书：

```bash
sing-box generate mitm-ca \
    --name "sing-box MITM CA" \
    --validity 3650 \
    --output ./mitm
```

输出：
```
mitm/
├── ca.pem   # 根证书（安装到 iOS）
└── ca.key   # 私钥（放在 sing-box 配置目录）
```

## 2. 安装 CA 证书到 iOS

### 方式一：通过文件安装

1. 将 `ca.pem` 发送到 iOS 设备（AirDrop、邮件、文件 App）
2. 在 iOS 上打开 `ca.pem`，系统会提示安装描述文件
3. 前往 **设置 → 通用 → VPN 与设备管理**，安装描述文件
4. 前往 **设置 → 通用 → 关于本机 → 证书信任设置**，启用对 `sing-box MITM CA` 的完全信任

### 方式二：通过越狱文件系统

```bash
# 将 ca.pem 复制到系统证书目录
cp ca.pem /var/Keychains/TrustStore.sqlite3  # 不推荐，需用工具注入

# 推荐使用方式一
```

## 3. 部署 sing-box 二进制

```bash
# 将 sing-box 二进制复制到 iOS 设备
scp sing-box root@<设备IP>:/var/jb/usr/bin/sing-box
ssh root@<设备IP> "chmod +x /var/jb/usr/bin/sing-box"
```

## 4. 配置文件

创建 `/var/jb/etc/sing-box/config.json`：

```json
{
  "log": {
    "level": "info",
    "output": "/var/log/sing-box.log"
  },
  "inbounds": [
    {
      "type": "tun",
      "tag": "tun-in",
      "interface_name": "sing-box0",
      "inet4_address": "172.19.0.1/30",
      "auto_route": true,
      "strict_route": true
    }
  ],
  "outbounds": [
    {
      "type": "direct",
      "tag": "direct"
    }
  ],
  "services": [
    {
      "type": "mitm",
      "tag": "mitm",
      "enabled": true,
      "ca": {
        "certificate": "/var/jb/etc/sing-box/ca.pem",
        "private_key": "/var/jb/etc/sing-box/ca.key"
      },
      "match": {
        "domain_suffix": ["example.com"]
      },
      "rewrite": {
        "enabled": true
      }
    }
  ]
}
```

将 `ca.pem` 和 `ca.key` 复制到 `/var/jb/etc/sing-box/`：

```bash
ssh root@<设备IP> "mkdir -p /var/jb/etc/sing-box"
scp ca.pem ca.key root@<设备IP>:/var/jb/etc/sing-box/
ssh root@<设备IP> "chmod 600 /var/jb/etc/sing-box/ca.key"
```

## 5. 配置 LaunchDaemon（开机自启）

```bash
# 复制 plist
scp com.sb1.mitm.plist root@<设备IP>:/var/jb/Library/LaunchDaemons/
ssh root@<设备IP> "chown root:wheel /var/jb/Library/LaunchDaemons/com.sb1.mitm.plist"

# 加载
ssh root@<设备IP> "launchctl load /var/jb/Library/LaunchDaemons/com.sb1.mitm.plist"

# 查看状态
ssh root@<设备IP> "launchctl list | grep sing-box"

# 查看日志
ssh root@<设备IP> "tail -f /var/log/sing-box.log"
```

## 6. 验证 MITM 工作

1. 在 iOS 设备上访问 `https://example.com`
2. 查看 sing-box 日志，应出现 `mitm: 域名命中` 等日志
3. 证书应显示为由 `sing-box MITM CA` 签发

## 7. 自研 App 调试信任

对于自研 App，推荐在 Debug 构建中信任 MITM CA：

```swift
// URLSessionDelegate 实现
func urlSession(_ session: URLSession,
                didReceive challenge: URLAuthenticationChallenge,
                completionHandler: @escaping (URLSession.AuthChallengeDisposition, URLCredential?) -> Void) {
    #if DEBUG
    // 调试版本信任 MITM CA
    if let serverTrust = challenge.protectionSpace.serverTrust {
        completionHandler(.useCredential, URLCredential(trust: serverTrust))
        return
    }
    #endif
    completionHandler(.performDefaultHandling, nil)
}
```

或在 Info.plist 中配置 `NSAppTransportSecurity`：

```xml
<key>NSAppTransportSecurity</key>
<dict>
    <key>NSAllowsArbitraryLoads</key>
    <true/>
</dict>
```

**注意：** 第三方 App 的证书绑定（Certificate Pinning）绕过不属于 sing-box MITM 核心功能，需要单独的越狱 Tweak 处理。

## 8. 故障排查

| 问题 | 原因 | 解决方案 |
| --- | --- | --- |
| 连接失败 | CA 未信任 | 检查证书信任设置 |
| 无 MITM 日志 | 域名未匹配 | 检查 match.domain_suffix 配置 |
| 启动失败 | 配置错误 | `sing-box check -c /var/jb/etc/sing-box/config.json` |
| TUN 无法创建 | 权限不足 | 确保以 root 运行 |
| 上游连接失败 | 路由配置 | 检查 outbounds 和 route 规则 |
