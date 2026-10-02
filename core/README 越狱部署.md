

# sing-box iOS 越狱部署文档

## 1. 文档目标

本文档说明基于 sing-box + MITM 扩展版本，在 iOS 越狱环境中的部署方式。

目标：

* 在越狱 iOS 设备运行 sing-box daemon。
* 启用 TUN 网络接管。
* 部署 MITM CA。
* 管理配置文件。
* 使用 LaunchDaemon 自动启动。
* 提供调试环境。
* 支持开发测试场景。

本文档针对：

* 越狱测试设备
* 自研 App 调试
* 网络分析环境
* sing-box 二次开发验证

不作为 App Store 发布方案。

---

# 2. 系统架构

最终运行结构：

```
iOS Device

launchd
 |
 |
 sing-box daemon
 |
 +----------------+
 |                |
 TUN Service      MITM Service
 |                |
 |                |
 Network          TLS Interception
 |
 Router
 |
 Outbound
 |
 Internet
```

目录：

```
/var/mobile/singbox/

├── sing-box
├── config.json
├── mitm/
│   ├── ca.pem
│   └── ca.key
├── cache/
└── logs/
```

---

# 3. 环境要求

## 3.1 设备要求

建议：

* 越狱 iPhone/iPad
* iOS 14+
* arm64 / arm64e

检查：

```bash
uname -m
```

输出：

```
arm64
```

---

## 3.2 权限要求

sing-box 需要：

* 创建 TUN interface
* 修改网络路由
* 读取配置
* 访问 CA 文件

运行用户：

推荐：

```
mobile
```

特殊网络操作由 jailbreak 环境提供。

---

# 4. 安装目录

创建：

```bash
mkdir -p /var/mobile/singbox
mkdir -p /var/mobile/singbox/mitm
mkdir -p /var/mobile/singbox/logs
mkdir -p /var/mobile/singbox/cache
```

权限：

```bash
chmod 700 /var/mobile/singbox
chmod 600 /var/mobile/singbox/mitm/*
```

---

# 5. 部署 sing-box 二进制

复制：

```
sing-box
```

到：

```
/var/mobile/singbox/sing-box
```

权限：

```bash
chmod 755 /var/mobile/singbox/sing-box
```

测试：

```bash
/var/mobile/singbox/sing-box version
```

输出：

```
sing-box version x.x.x
```

---

# 6. 配置文件部署

位置：

```
/var/mobile/singbox/config.json
```

示例：

```json
{
  "log": {
    "level": "info"
  },

  "inbounds": [
    {
      "type": "tun",
      "tag": "tun0",
      "interface_name": "utun9",
      "auto_route": true
    }
  ],

  "services": [
    {
      "type": "mitm",
      "tag": "mitm",
      "options": {
        "enabled": true,

        "ca": {
          "certificate": "/var/mobile/singbox/mitm/ca.pem",
          "private_key": "/var/mobile/singbox/mitm/ca.key"
        }
      }
    }
  ]
}
```

检查：

```bash
sing-box check \
-c /var/mobile/singbox/config.json
```

---

# 7. MITM CA 部署

目录：

```
mitm/

ca.pem
ca.key
```

其中：

## ca.pem

Root CA 公钥证书。

用于：

* 签发动态 HTTPS certificate
* 安装到测试设备信任列表

---

## ca.key

Root CA 私钥。

必须保护：

```
chmod 600 ca.key
```

禁止：

* 上传
* 日志输出
* 调试打印

---

# 8. iOS CA 信任

## 8.1 用户级安装

生成：

```
ca.pem
```

转换：

```
ca.cer
```

安装：

Safari 打开：

```
ca.cer
```

然后：

设置：

```
设置
 ↓
通用
 ↓
VPN与设备管理
 ↓
安装描述文件
```

---

## 8.2 启用完全信任

安装后：

```
设置
 ↓
通用
 ↓
关于本机
 ↓
证书信任设置
```

开启：

```
MITM Root CA
```

---

# 9. LaunchDaemon 自动启动

创建：

```
/Library/LaunchDaemons/com.singbox.daemon.plist
```

内容：

```xml
<?xml version="1.0" encoding="UTF-8"?>

<plist version="1.0">

<dict>

<key>Label</key>
<string>com.singbox.daemon</string>


<key>ProgramArguments</key>

<array>

<string>/var/mobile/singbox/sing-box</string>

<string>run</string>

<string>-c</string>

<string>/var/mobile/singbox/config.json</string>

</array>


<key>RunAtLoad</key>
<true/>


<key>KeepAlive</key>
<true/>


<key>StandardOutPath</key>

<string>/var/mobile/singbox/logs/stdout.log</string>


<key>StandardErrorPath</key>

<string>/var/mobile/singbox/logs/error.log</string>


</dict>

</plist>
```

---

# 10. 加载服务

权限：

```bash
chmod 644 \
/Library/LaunchDaemons/com.singbox.daemon.plist
```

加载：

```bash
launchctl load \
/Library/LaunchDaemons/com.singbox.daemon.plist
```

查看：

```bash
launchctl list | grep singbox
```

停止：

```bash
launchctl unload \
/Library/LaunchDaemons/com.singbox.daemon.plist
```

---

# 11. 网络启动流程

启动后：

```
launchd

↓

sing-box

↓

initialize config

↓

initialize TUN

↓

initialize Router

↓

initialize MITM Service

↓

ready
```

日志：

```
[INFO] tun started
[INFO] router initialized
[INFO] mitm service started
```

---

# 12. TUN 配置注意事项

iOS 网络环境特殊：

需要处理：

* utun interface
* route table
* DNS interception
* IPv4
* IPv6

建议：

开发阶段：

```
strict_route=false
```

稳定后：

```
strict_route=true
```

---

# 13. MITM 测试流程

测试目标：

```
App

 |

HTTPS

 |

TUN

 |

MITM

 |

Router

 |

Outbound

 |

Server
```

测试：

访问：

```
https://example.com
```

日志：

应该看到：

```
[MITM]
ClientHello received

SNI:
example.com

certificate issued

HTTP request intercepted
```

---

# 14. 自研 App 调试

推荐：

Debug Build：

```
允许测试 CA
```

Release Build：

```
关闭 MITM 信任
```

结构：

```
App Debug

URLSession

   |

Trust Evaluation

   |

Allow Test CA
```

不要修改核心网络逻辑。

---

# 15. App Transport Security

开发测试：

可以配置：

```
Info.plist
```

例如：

```
NSAppTransportSecurity
```

但是：

不要长期关闭 ATS。

建议：

只针对测试环境。

---

# 16. 日志管理

日志目录：

```
/var/mobile/singbox/logs/
```

建议：

```
singbox.log
error.log
```

生产环境禁止：

记录：

* Cookie
* Authorization
* Token
* 用户数据
* 请求 Body

---

# 17. 性能优化

MITM 开销：

```
TLS decrypt
+
TLS encrypt
+
HTTP parsing
```

优化：

## 不修改 Body

使用：

```
stream relay
```

---

## 修改 Body

才：

```
buffer
↓
rewrite
↓
compress
```

---

# 18. 常见问题

## 1. HTTPS 打不开

检查：

```
CA 是否信任
```

查看：

```
证书信任设置
```

---

## 2. MITM 没有日志

检查：

配置：

```json
{
 "enabled": true
}
```

检查：

```
domain match
```

---

## 3. App 拒绝连接

可能原因：

* App 自带 certificate pinning
* App 使用自定义 TLS
* App 使用特殊网络栈

需要在 App Debug 环境调整信任策略。

---

## 4. TUN 无流量

检查：

```
utun interface
```

检查：

```
route table
```

检查：

```
DNS
```

---

# 19. 安全建议

必须：

* 单独生成测试 CA。
* 不复用生产证书。
* 保护 ca.key。
* 不开放远程控制接口。
* 定期删除测试 CA。

推荐：

开发设备：

```
MITM CA A
```

生产设备：

```
MITM disabled
```

---

# 20. 发布流程

推荐：

```
代码修改

↓

Go build

↓

device test

↓

install binary

↓

deploy config

↓

install CA

↓

launch daemon

↓

integration test

↓

release
```

---

# 21. 项目维护目录

最终：

```
/var/mobile/singbox/

├── sing-box
├── config.json
│
├── mitm/
│   ├── ca.pem
│   └── ca.key
│
├── cache/
│
└── logs/
    ├── stdout.log
    └── error.log
```

---

# 22. 生产化检查清单

完成：

* [ ] sing-box daemon 自动启动
* [ ] TUN 正常工作
* [ ] Router 正常分流
* [ ] MITM Service 加载
* [ ] CA 正常签发
* [ ] HTTPS interception 正常
* [ ] HTTP/2 正常
* [ ] WebSocket 正常
* [ ] Rewrite 正常
* [ ] 日志脱敏
* [ ] 性能测试完成

---

# 23. 总结

iOS 越狱部署层只负责：

```
设备环境
+
启动管理
+
文件管理
+
CA 安装
```

核心网络能力仍然属于：

```
sing-box core

TUN
Router
MITM
Outbound
```

保持平台层和核心层分离，可以保证：

* Android 可复用
* macOS 可复用
* Linux 可复用
* iOS jailbreak 只是部署适配层

最终目标：

```
sing-box core
        +
MITM Service
        +
iOS jailbreak launcher
        =
完整 iOS HTTPS 调试代理平台
```
