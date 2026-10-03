# iOS 越狱设备实测指南

## 前置条件

- iOS 越狱设备（arm64，iOS 15+）
- 已安装 OpenSSH（通过 Cydia/Sileo）
- 电脑与 iOS 设备在同一局域网
- 已知 iOS 设备的 IP 地址和 root 密码

## 一、传输部署包到 iOS 设备

### 方式一：SCP 传输

```bash
# 在电脑上执行
scp -r MITM-ios-deploy root@<设备IP>:/var/root/
```

### 方式二：通过文件 App

将整个 `MITM-ios-deploy` 目录压缩为 zip，通过 AirDrop 或邮件发送到 iOS 设备，解压到 `/var/root/`。

## 二、安装

```bash
# SSH 登录到 iOS 设备
ssh root@<设备IP>

# 进入部署目录
cd /var/root/MITM-ios-deploy

# 运行安装脚本
chmod +x install.sh
./install.sh
```

安装脚本会自动完成：
1. 安装 sing-box 二进制到 `/var/jb/usr/bin/sing-box`
2. 创建配置目录 `/var/jb/etc/sing-box/`
3. 生成 MITM CA 证书
4. 安装 LaunchDaemon 到 `/var/jb/Library/LaunchDaemons/`
5. 验证配置文件

## 三、安装 CA 证书到系统信任存储

### 3.1 导出 CA 证书到可访问位置

```bash
# 在 iOS 设备上执行
cp /var/jb/etc/sing-box/ca.pem /var/mobile/Documents/
chown mobile:mobile /var/mobile/Documents/ca.pem
```

### 3.2 安装描述文件

1. 打开「文件」App，找到 `/var/mobile/Documents/ca.pem`
2. 点击证书文件，系统会提示「已下载描述文件」
3. 打开 **设置 → 通用 → VPN 与设备管理**
4. 找到「sing-box MITM CA」描述文件，点击安装
5. 输入设备密码确认

### 3.3 启用证书信任

1. 打开 **设置 → 通用 → 关于本机 → 证书信任设置**
2. 找到「sing-box MITM CA」，启用完全信任
3. 确认警告对话框

## 四、启动服务

```bash
# 加载 LaunchDaemon（开机自启）
launchctl load /var/jb/Library/LaunchDaemons/com.sb1.mitm.plist

# 查看服务状态
launchctl list | grep sing-box

# 查看日志
tail -f /var/log/sing-box.log
```

预期日志输出：
```
infra: sing-box version 1.x.x
mitm: 服务初始化完成，根证书已加载
mitm: 重写引擎已启用，规则数: 1
mitm: 服务已启动
inbound/tun: started at sing-box0
```

## 五、验证 MITM 工作

### 5.1 基础连通性测试

在 iOS 设备上打开 Safari，访问 `https://example.com`

### 5.2 查看 MITM 拦截日志

```bash
grep "mitm:" /var/log/sing-box.log
```

预期输出：
```
mitm: 开始拦截连接，来源: 172.19.0.1:xxxxx 目标: 93.184.216.34:443
mitm: ClientHello SNI=example.com ALPN=[h2 http/1.1]
mitm: 域名命中，开始 TLS 终止: example.com
mitm: 客户端 TLS 握手完成，协议: h2
mitm: 上游路由决策，目标域名: example.com
```

### 5.3 验证证书签发

在 Safari 中访问 `https://example.com`，点击地址栏的锁图标，查看证书信息：
- 颁发者（Issuer）应为 `sing-box MITM CA`
- 主题（Subject）应为 `example.com`

### 5.4 验证 Rewrite 生效

默认配置会给 `example.com` 的响应添加 `X-MITM: sing-box` 响应头。

可以用 curl 验证（需要在 iOS 上安装 curl，或用其他 HTTP 客户端）：

```bash
curl -vI https://example.com 2>&1 | grep -i "x-mitm"
```

预期输出：
```
< x-mitm: sing-box
```

## 六、自定义配置

### 6.1 修改匹配域名

编辑 `/var/jb/etc/sing-box/config.json`：

```json
"match": {
  "domain_suffix": ["your-domain.com", "another-domain.com"]
}
```

### 6.2 添加 Rewrite 规则

```json
"rewrite": {
  "enabled": true,
  "rules": [
    {
      "domain_suffix": ["example.com"],
      "path_prefix": "/api/",
      "request_header": {"X-Debug": "1"},
      "response_header": {"X-Rewritten": "true"},
      "body_replace": [{"find": "old-text", "replace": "new-text"}]
    }
  ]
}
```

### 6.3 配置代理出站

```json
"outbounds": [
  {
    "type": "socks",
    "tag": "proxy",
    "server": "127.0.0.1",
    "server_port": 1080
  },
  {
    "type": "direct",
    "tag": "direct"
  }
],
"route": {
  "rules": [
    {
      "domain_suffix": ["example.com"],
      "outbound": "proxy"
    }
  ]
}
```

修改配置后重启服务：
```bash
launchctl unload /var/jb/Library/LaunchDaemons/com.sb1.mitm.plist
launchctl load /var/jb/Library/LaunchDaemons/com.sb1.mitm.plist
```

## 七、故障排查

| 问题 | 可能原因 | 解决方案 |
| --- | --- | --- |
| 服务启动失败 | 配置错误 | `sing-box check -c /var/jb/etc/sing-box/config.json` |
| 无 MITM 日志 | 域名未匹配 | 检查 `match.domain_suffix` 配置 |
| 证书不受信任 | CA 未安装/未启用信任 | 重新执行第三节步骤 |
| TUN 无法创建 | 权限不足 | 确保以 root 运行，检查越狱环境 |
| 上游连接失败 | 网络/路由问题 | 检查 outbounds 和 route 配置 |
| 连接被重置 | TLS 握手失败 | 检查 CA 证书和域名匹配 |
| 性能差 | Body 重写缓冲大 | 减小 `max_body_size` 或减少 body_replace 规则 |

### 7.1 查看详细日志

```bash
# 修改配置中的 log.level 为 "debug"
# 重启服务后查看详细日志
tail -f /var/log/sing-box.log
```

### 7.2 手动运行（前台模式，便于调试）

```bash
# 先停止 LaunchDaemon
launchctl unload /var/jb/Library/LaunchDaemons/com.sb1.mitm.plist

# 前台运行
sing-box run -c /var/jb/etc/sing-box/config.json
```

### 7.3 验证二进制兼容性

```bash
# 检查二进制是否能在当前设备运行
sing-box version

# 如果出现 "killed: 9" 或代码签名错误，需要：
# 1. 使用 ldid 进行伪签名
ldid -S /var/jb/usr/bin/sing-box
# 2. 或在越狱环境中安装 AppSync Unified
```

## 八、卸载

```bash
# 停止服务
launchctl unload /var/jb/Library/LaunchDaemons/com.sb1.mitm.plist

# 删除文件
rm /var/jb/usr/bin/sing-box
rm -rf /var/jb/etc/sing-box
rm /var/jb/Library/LaunchDaemons/com.sb1.mitm.plist
rm /var/log/sing-box.log /var/log/sing-box.err.log

# 删除 CA 证书（设置 → 通用 → VPN 与设备管理 → 删除描述文件）
```

## 九、安全注意事项

1. **CA 私钥安全**：`/var/jb/etc/sing-box/ca.key` 权限为 0600，仅 root 可读
2. **匹配范围**：仅匹配必要的域名，避免解密所有 HTTPS 流量
3. **生产环境**：不建议在生产设备上长期运行 MITM
4. **法律合规**：仅在授权的设备上使用，遵守当地法律法规
