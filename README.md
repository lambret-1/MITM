




# sing-box iOS 越狱版 HTTPS MITM

## 完整开发设计与实现文档

**目标版本：** sing-box `testing` 分支
**目标平台：** iOS 越狱环境
**部署系统**最低支持 iOS16 系统
**主要语言：** Go / Swift / Objective-C（iOS 辅助层）
**核心能力：**

```text
iOS App
   │
   │ HTTPS
   ▼
TUN
   │
   │ TCP
   ▼
MITM Interception
   │
   ├── TLS ClientHello
   ├── SNI
   ├── Dynamic Certificate
   └── TLS Termination
          │
          ▼
      HTTP/1.1
      HTTP/2
      WebSocket
          │
          ▼
      Rewrite
          │
          ▼
   sing-box Router
          │
          ▼
     Outbound
          │
          ▼
     Real Server
```

---

# 1. 项目目标

本项目的目标不是简单增加一个 HTTP Proxy，而是在 sing-box 现有网络处理体系中加入一个完整的 HTTPS interception / MITM 服务。

最终要求：

1. 从 TUN 捕获 TCP 流量。
2. 对目标 HTTPS 连接进行选择性 interception。
3. 从 TLS ClientHello 获取 SNI。
4. 使用本地 CA 动态签发目标域名证书。
5. 与客户端建立 TLS。
6. 与真实服务器建立第二条 TLS。
7. 支持 HTTP/1.1。
8. 支持 HTTP/2。
9. 支持 WebSocket。
10. 对 HTTP request/response 执行 rewrite。
11. 解密后的请求重新进入 sing-box Router。
12. Router 决定 Direct / Proxy / Selector 等 outbound。
13. 不复制 sing-box route 规则。
14. iOS 越狱环境提供 CA 部署和服务启动能力。
15. 自研 App / Debug App 可以配置为信任测试 CA。

---

# 2. 总体架构

推荐最终架构：

```text
                       ┌─────────────────────┐
                       │      iOS App        │
                       └──────────┬──────────┘
                                  │
                               HTTPS
                                  │
                       ┌──────────▼──────────┐
                       │       TUN           │
                       │   L3 / L4 capture   │
                       └──────────┬──────────┘
                                  │
                           TCP connection
                                  │
                       ┌──────────▼──────────┐
                       │ MITM Interceptor    │
                       └──────────┬──────────┘
                                  │
                         TLS ClientHello
                                  │
                                 SNI
                                  │
                       ┌──────────▼──────────┐
                       │   MITM Certificate  │
                       │       Engine        │
                       └──────────┬──────────┘
                                  │
                           TLS termination
                                  │
                ┌─────────────────┴─────────────────┐
                │                                   │
         HTTP/1.1                             HTTP/2
                │                                   │
                └─────────────────┬─────────────────┘
                                  │
                             WebSocket
                                  │
                       ┌──────────▼──────────┐
                       │  Rewrite Engine     │
                       └──────────┬──────────┘
                                  │
                       ┌──────────▼──────────┐
                       │  sing-box Router    │
                       └──────────┬──────────┘
                                  │
                       ┌──────────▼──────────┐
                       │     Outbound        │
                       └──────────┬──────────┘
                                  │
                              Internet
```

核心职责严格分离：

| 模块                  | 职责               |
| ------------------- | ---------------- |
| TUN                 | L3/L4 捕获         |
| MITM                | TLS interception |
| HTTP engine         | HTTP/1.1、HTTP/2  |
| WebSocket           | Upgrade / 双向流    |
| Rewrite             | 修改 HTTP 数据       |
| Router              | 分流决策             |
| Outbound            | 实际建立外部连接         |
| iOS jailbreak layer | CA、启动、系统环境       |
| App debug layer     | 自研 App 测试信任      |

---

# 3. 为什么 MITM 应该作为 Service

早期设计考虑过：

```json
{
  "mitm": {}
}
```

但当前 `testing` 分支已经有比较完整的 Service Registry / Service Manager 架构。

因此最终推荐：

```json
{
  "services": [
    {
      "type": "mitm",
      "tag": "mitm",
      "options": {
        "enabled": true
      }
    }
  ]
}
```

架构：

```text
JSON
 │
 ▼
option.Options
 │
 ▼
Service options
 │
 ▼
Service Registry
 │
 ▼
mitm.NewService()
 │
 ▼
adapter.Service
 │
 ▼
Service Manager
```

当前 `testing` 已经通过 Service Registry 管理长期运行服务，因此 MITM 不应该另外创造一套生命周期。

---

# 4. 配置设计

## 4.1 完整配置示例

```json
{
  "log": {
    "level": "info"
  },

  "inbounds": [
    {
      "type": "tun",
      "tag": "tun-in",
      "interface_name": "utun9",
      "inet4_address": "172.19.0.1/30",
      "auto_route": true,
      "strict_route": true
    }
  ],

  "outbounds": [
    {
      "type": "direct",
      "tag": "direct"
    },

    {
      "type": "socks",
      "tag": "proxy",
      "server": "127.0.0.1",
      "server_port": 1080
    }
  ],

  "route": {
    "rules": [
      {
        "domain_suffix": [
          "example.com"
        ],
        "outbound": "proxy"
      }
    ]
  },

  "services": [
    {
      "type": "mitm",
      "tag": "mitm",
      "options": {
        "enabled": true,

        "ca": {
          "certificate": "/var/mobile/singbox/mitm/ca.pem",
          "private_key": "/var/mobile/singbox/mitm/ca.key"
        },

        "match": {
          "domain_suffix": [
            "example.com",
            "example.net"
          ]
        },

        "rewrite": {
          "enabled": true
        }
      }
    }
  ]
}
```

---

# 5. MITM 与 Route 的职责

必须保持：

```text
MITM match
     │
     ▼
是否需要 HTTPS 解密
```

而：

```text
route.rules
     │
     ▼
解密后应该从哪个 outbound 访问服务器
```

例如：

```json
"match": {
  "domain_suffix": [
    "example.com"
  ]
}
```

表示：

```text
example.com
      ↓
MITM
```

而：

```json
"route": {
  "rules": [
    {
      "domain_suffix": [
        "example.com"
      ],
      "outbound": "proxy"
    }
  ]
}
```

表示：

```text
example.com
      ↓
proxy outbound
```

不要把：

```json
"mitm": {
  "rules": [
    {
      "domain": "example.com",
      "outbound": "proxy"
    }
  ]
}
```

再实现一遍。

否则最终会出现两套 routing engine。

---

# 6. Option 结构

推荐：

```go
type MITMServiceOptions struct {
    Enabled bool `json:"enabled,omitempty"`

    CA MITMCAOptions `json:"ca,omitempty"`

    Match MITMMatchOptions `json:"match,omitempty"`

    Rewrite MITMRewriteOptions `json:"rewrite,omitempty"`
}

type MITMCAOptions struct {
    Certificate string `json:"certificate"`

    PrivateKey string `json:"private_key"`
}

type MITMMatchOptions struct {
    Domain []string `json:"domain,omitempty"`

    DomainSuffix []string `json:"domain_suffix,omitempty"`
}

type MITMRewriteOptions struct {
    Enabled bool `json:"enabled,omitempty"`
}
```

后续可以扩展：

```go
type MITMMatchOptions struct {
    Domain       []string `json:"domain,omitempty"`
    DomainSuffix []string `json:"domain_suffix,omitempty"`
    DomainRegex  []string `json:"domain_regex,omitempty"`

    Port []uint16 `json:"port,omitempty"`
}
```

但第一版不要过度设计。

---

# 7. Service Registry

当前 sing-box 架构中，MITM 应该注册成：

```go
service.Register[option.MITMServiceOptions](
    registry,
    constant.TypeMITM,
    NewService,
)
```

其中：

```go
const TypeMITM = "mitm"
```

最终：

```text
services[]
    │
    ▼
type == "mitm"
    │
    ▼
MITMServiceOptions
    │
    ▼
NewService()
```

---

# 8. MITM Service

建议结构：

```go
type Service struct {
    ctx context.Context

    logger log.ContextLogger

    router adapter.Router

    options option.MITMServiceOptions

    ca *CA

    matcher *Matcher

    certificates *CertificateCache

    rewrite *RewriteEngine
}
```

初始化：

```text
NewService()
   │
   ├── Load CA
   ├── Validate CA
   ├── Create matcher
   ├── Create certificate cache
   ├── Create rewrite engine
   └── Obtain Router
```

这里必须使用现有 sing-box Router。

不能创建第二个 Router。

---

# 9. Router 获取

正确思路：

```text
Box
 │
 ├── Router
 │
 ├── Inbound Manager
 │
 ├── Outbound Manager
 │
 └── Service Manager
         │
         └── MITM
```

MITM 使用现有 Router：

```go
router := service.FromContext[adapter.Router](ctx)
```

实际代码需要根据当前 commit 的 context helper API 调整。

核心原则不变：

> MITM 永远不要自己重新创建一套 Router。

---

# 10. TUN 层

TUN 的职责保持：

```text
IP
 ↓
TCP
 ↓
connection
```

不应该把：

```text
TLS
HTTP
WebSocket
Rewrite
```

全部塞进 `protocol/tun/inbound.go`。

推荐抽象：

```go
type Interceptor interface {
    ShouldIntercept(metadata Metadata) bool

    Intercept(
        ctx context.Context,
        conn net.Conn,
        metadata Metadata,
    error
}
```

TUN：

```go
if interceptor != nil &&
    interceptor.ShouldIntercept(metadata) {

    return interceptor.Intercept(
        ctx,
        conn,
        metadata,
    )
}
```

这样 TUN 不需要知道 MITM 的内部实现。

---

# 11. HTTPS interception

处理流程：

```text
TCP connection
      │
      ▼
Read TLS ClientHello
      │
      ▼
Extract SNI
      │
      ▼
MITM matcher
      │
      ├── no match → normal routing
      │
      └── match
             │
             ▼
       generate leaf cert
             │
             ▼
       tls.Server()
             │
             ▼
      decrypted connection
```

---

# 12. SNI 提取

不要直接假设：

```text
TCP dst port == 443
```

就一定是 HTTPS。

第一阶段应该：

```text
TCP
 ↓
ClientHello
 ↓
SNI
 ↓
ALPN
```

读取：

```text
server_name
supported_versions
ALPN
cipher suites
```

重点字段：

```text
SNI = example.com
ALPN = h2,http/1.1
```

---

# 13. Dynamic Certificate

CA：

```text
Root CA
   │
   ├── example.com
   ├── api.example.com
   ├── www.example.net
   └── ...
```

每个 domain 生成 leaf certificate。

建议 cache：

```go
type CertificateCache struct {
    mu sync.RWMutex

    certificates map[string]*tls.Certificate
}
```

使用：

```go
cert, err := cache.Get("example.com")
```

如果没有：

```text
generate certificate
      │
      ▼
cache
      │
      ▼
return
```

---

# 14. Certificate 属性

生成 leaf certificate 时：

```text
Subject:
    CN = example.com

DNSNames:
    example.com

ExtKeyUsage:
    ServerAuth

KeyUsage:
    DigitalSignature
```

签发者：

```text
Issuer = MITM CA
```

而不是直接使用真实服务器 certificate。

---

# 15. 客户端 TLS

：

```go
tls.Server(
    clientConn,
    &tls.Config{
        Certificates: []tls.Certificate{
            *leafCert,
        },
    },
)
```

握手完成：

```text
iOS App
   │
 TLS
   │
   ▼
MITM
```

MITM 已经获得明文 HTTP。

---

# 16. 上游 TLS

MITM 不应该：

```go
net.Dial("tcp", host)
```

然后绕过 sing-box Router。

正确：

```text
HTTP request
     │
     ▼
Router
     │
     ▼
Outbound
     │
     ▼
TCP connection
     │
     ▼
TLS Client
     │
     ▼
Real server
```

TLS：

```go
tls.Client(
    outboundConn,
    &tls.Config{
        ServerName: domain,
    },
)
```

---

# 17. HTTP/1.1

HTTP/1.1 处理：

```text
client TLS
    │
    ▼
http.ReadRequest
    │
    ▼
request rewrite
    │
    ▼
Router
    │
    ▼
upstream
    │
    ▼
response
    │
    ▼
response rewrite
    │
    ▼
client
```

需要处理：

* Host
* URL
* Method
* Header
* Cookie
* Content-Length
* Transfer-Encoding
* gzip
* br
* zstd
* chunked
* trailers
* connection reuse

---

# 18. HTTP/2

HTTP/2 不能简单当 HTTP/1.1：

```go
http.ReadRequest(...)
```

因为 HTTP/2：

```text
connection
   │
   ├── stream 1
   ├── stream 3
   ├── stream 5
   └── stream 7
```

MITM engine 必须支持：

```text
HTTP/2 connection
       │
       ├── request stream
       ├── response stream
       └── stream lifecycle
```

并保持：

```text
HPACK
HEADERS
DATA
RST_STREAM
WINDOW_UPDATE
SETTINGS
```

第一版可以优先使用 `golang.org/x/net/http2`。

不要自行实现 HTTP/2 framing。

---

# 19. ALPN

客户端可能发送：

```text
ALPN:
    h2
    http/1.1
```

MITM 应根据 ALPN 决定协议：

```text
h2
 │
 ▼
HTTP/2 engine

http/1.1
 │
 ▼
HTTP/1 engine
```

上游连接也应该根据服务器支持情况协商。

---

# 20. WebSocket

典型：

```text
GET /socket HTTP/1.1
Connection: Upgrade
Upgrade: websocket
```

握手完成以后：

```text
HTTP
 │
 ▼
101 Switching Protocols
 │
 ▼
raw bidirectional stream
```

此时：

```text
client ⇄ MITM ⇄ upstream
```

不能继续把 WebSocket frame 当普通 HTTP body。

最基本的 relay：

```go
go io.Copy(upstream, client)
go io.Copy(client, upstream)
```

生产版本需要处理：

* half close
* context cancellation
* close frames
* backpressure
* deadlines

---

# 21. Rewrite Engine

建议独立：

```text
protocol/mitm/rewrite/
    engine.go
    rule.go
    matcher.go
```

结构：

```go
type Rule struct {
    DomainSuffix []string

    PathPrefix string

    Method []string

    RequestHeader map[string]string

    ResponseHeader map[string]string

    BodyReplace []ReplaceRule
}
```

例如：

```json
{
  "domain_suffix": [
    "example.com"
  ],
  "path_prefix": "/api/",
  "response_header": {
    "X-Debug": "1"
  }
}
```

---

# 22. Body Rewrite

处理：

```text
response
   │
   ▼
Content-Encoding
   │
   ├── identity
   ├── gzip
   ├── br
   └── zstd
```

正确流程：

```text
compressed body
       │
       ▼
decompress
       │
       ▼
rewrite
       │
       ▼
compress
       │
       ▼
update headers
```

必须正确更新：

```text
Content-Length
Content-Encoding
Transfer-Encoding
```

不能简单：

```go
bytes.ReplaceAll(body)
```

然后直接把原来的 `Content-Length` 留着。

---

# 23. Router Integration

这是整个项目最重要的部分。

MITM 解密：

```text
HTTPS
 ↓
HTTP request
```

之后构造 sing-box metadata：

```text
network = tcp
source = client
destination = server
domain = example.com
port = 443
```

然后：

```text
MITM
 │
 ▼
Router.RouteConnection(...)
 │
 ▼
Outbound
```

---

# 24. 为什么不能 MITM 自己选择 Proxy

错误：

```go
if domain == "example.com" {
    dial("proxy")
}
```

正确：

```text
MITM
 │
 ▼
sing-box Router
 │
 ├── rule 1
 ├── rule 2
 ├── rule-set
 ├── geoip
 ├── geosite
 └── final
 │
 ▼
Outbound
```

这样 MITM 不需要理解：

```text
rule-set
DNS rule
geoip
geosite
selector
urltest
```

---

# 25. Connection Context

MITM 需要保留：

```text
Inbound
Source
Destination
Domain
Port
Network
Process metadata
```

如果 iOS/TUN 层能取得进程相关信息，则应该保留到 Router。

最终：

```text
TUN metadata
      │
      ▼
MITM
      │
      ▼
HTTP metadata
      │
      ▼
Router
```

而不是 MITM 重新创建一个完全不同的 context。

---

# 26. MITM 不匹配时

如果：

```text
SNI = google.com
```

而配置：

```json
"domain_suffix": [
    "example.com"
]
```

则：

```text
MITM match = false
```

应继续普通 sing-box routing。

即：

```text
TCP
 │
 ├── MITM match?
 │       │
 │       ├── false → normal route
 │       │
 │       └── true → MITM
```

---

# 27. 明确的 bypass 情况

第一版应该允许：

```text
non-TLS
TLS without SNI
unsupported protocol
MITM disabled
domain not matched
certificate generation failure
```

回退到：

```text
normal sing-box routing
```

不要因为 MITM 无法处理就直接破坏整个 TCP connection，除非配置明确要求 fail-closed。

---

# 28. iOS 越狱层

建议独立：

```text
platform/apple/jailbreak/
    ca/
    launcher/
    trust/
```

核心职责：

```text
sing-box
   │
   ├── configuration
   ├── MITM CA
   └── daemon lifecycle
```

---

# 29. CA 文件

建议：

```text
/var/mobile/singbox/
    config.json

    mitm/
        ca.pem
        ca.key

    cache/

    logs/
```

权限：

```text
ca.key
```

必须限制访问。

例如：

```text
owner = mobile/root
mode = 0600
```

具体权限应根据 jailbreak 环境和 daemon 运行用户调整。

---

# 30. LaunchDaemon

越狱环境可以采用：

```text
/Library/LaunchDaemons/
    com.example.singbox.plist
```

启动：

```text
launchd
   │
   ▼
sing-box
   │
   ▼
TUN
   │
   ▼
MITM Service
```

配置路径不要硬编码到 Go core 中。

iOS layer 负责传入：

```text
--config
```

---

# 31. Certificate Trust

测试环境需要：

```text
MITM Root CA
       │
       ▼
iOS trust store / App trust
       │
       ▼
TLS client accepts generated certificate
```

但是必须区分：

### 自研 App

推荐：

```text
DEBUG
 │
 └── Trust test CA
```

或者：

```text
Debug build
    ↓
disable pinning
```

### 第三方 App

不要把通用的：

```text
NSURLSession hook
BoringSSL hook
certificate pinning bypass
```

作为核心 sing-box 功能。

这是 App 自身的安全边界问题，而且会让 MITM core 和 iOS jailbreak hack 强耦合。

---

# 32. 自研 App 推荐方案

例如 App 自己控制：

```swift
#if DEBUG
let allowMITM = true
#else
let allowMITM = false
#endif
```

或者：

```text
Debug certificate authority
       │
       ▼
URLSession delegate
       │
       ▼
trust test CA
```

生产 build：

```text
MITM disabled
```

这样最容易维护。

---

# 33. CLI

配置检查：

```bash
sing-box check -c config.json
```

预期：

```text
configuration is valid
```

MITM CA 工具：

```bash
sing-box generate mitm-ca \
    --name "sing-box Debug CA" \
    --validity 3650 \
    --output ./mitm
```

输出：

```text
mitm/
├── ca.pem
└── ca.key
```

实际 CLI 参数需要根据当前 `cmd/sing-box` / `generate` 命令体系接入，而不是创建第二套 command framework。

---

# 34. Config Schema

不要手工维护另一套 JSON schema。

使用 sing-box 现有 schema generation：

```text
option.Options
       │
       ▼
reflection
       │
       ▼
schema.Generate()
```

MITM 的：

```go
json:"..."
```

字段自动进入 schema。

这样：

```text
Go struct
     │
     ├── parser
     ├── validation
     └── schema
```

可以保持一致。

---

# 35. 单元测试

至少：

```text
test/
    mitm_config_test.go
    mitm_match_test.go
    mitm_cert_test.go
    mitm_http_test.go
    mitm_router_test.go
```

测试矩阵：

| 测试           | 目标               |
| ------------ | ---------------- |
| Config Parse | JSON → Options   |
| Registry     | `type=mitm` 正确注册 |
| CA           | CA 加载            |
| Certificate  | 动态证书             |
| Matcher      | domain 匹配        |
| TLS          | ClientHello/SNI  |
| HTTP/1.1     | request/response |
| HTTP/2       | streams          |
| WebSocket    | upgrade          |
| Rewrite      | body/header      |
| Router       | MITM → route     |
| Outbound     | route → outbound |

---

# 36. Integration Test

推荐建立 fake server：

```text
                 ┌───────────────┐
                 │ Fake HTTPS    │
                 │ Server        │
                 └───────▲───────┘
                         │
                    TLS upstream
                         │
                 ┌───────┴───────┐
                 │     MITM      │
                 └───────▲───────┘
                         │
                    TLS client
                         │
                 ┌───────┴───────┐
                 │ Test Client   │
                 └───────────────┘
```

测试：

```text
client
 ↓
TLS
 ↓
MITM
 ↓
rewrite
 ↓
router
 ↓
fake outbound
 ↓
server
```

---

# 37. 推荐测试用例

### Case 1

```text
GET /hello
```

预期：

```text
200
```

### Case 2

Response：

```text
hello world
```

rewrite：

```text
hello singbox
```

预期：

```text
hello singbox
```

### Case 3

：

```text
HTTP/2 GET
```

预期：

```text
HTTP/2 200
```

### Case 4

：

```text
WebSocket
```

预期：

```text
101 Switching Protocols
```

### Case 5

route：

```text
example.com → proxy
```

预期：

```text
MITM
 ↓
Router
 ↓
proxy
```

而不是：

```text
MITM
 ↓
direct
```

---

# 38. 性能考虑

MITM 最大开销：

```text
TLS termination
+
TLS upstream
+
HTTP parsing
+
body buffering
```

因此：

```text
不 rewrite body
```

时尽可能：

```text
streaming
```

而：

```text
需要 body rewrite
```

时才：

```text
buffer
```

---

# 39. Body Size Limit

绝对不要：

```go
io.ReadAll(resp.Body)
```

无限读取。

应该：

```text
max_body_size
```

例如：

```json
"rewrite": {
  "enabled": true,
  "max_body_size": 10485760
}
```

超过：

```text
10 MB
```

可以：

```text
skip rewrite
```

或者：

```text
fail
```

由配置决定。

---

# 40. Certificate Cache Limit

不能无限：

```text
map[string]*tls.Certificate
```

因为长期运行可能出现：

```text
100,000 domains
```

建议：

```text
LRU
```

例如：

```text
capacity = 4096
```

同时支持：

```text
TTL
```

---

# 41. 日志

建议：

```text
[MITM] SNI example.com
[MITM] certificate generated
[MITM] HTTP/2 stream
[MITM] route outbound=proxy
[MITM] rewrite response
```

但是禁止：

```text
private key
cookies
authorization
full body
```

默认进入日志。

Debug 模式才允许：

```text
request headers
response headers
```

并且应该显式启用。

---

# 42. 安全模型

MITM CA private key 是整个系统最敏感的数据之一。

必须：

```text
0600
```

并避免：

```text
log
stdout
panic
debug dump
```

泄漏。

同时：

```text
CA
```

最好和普通 outbound credentials 分开存储。

---

# 43. Fail-open / Fail-closed

建议配置：

```json
{
  "mitm": {
    "on_error": "bypass"
  }
}
```

两种模式：

```text
bypass
```

MITM 失败：

```text
→ normal route
```

或者：

```text
block
```

MITM 失败：

```text
→ connection closed
```

第一版默认建议使用明确的配置语义，而不是隐藏行为。

---

# 44. 推荐源码结构

最终建议：

```text
protocol/mitm/
├── service.go
├── interceptor.go
├── matcher.go
├── clienthello.go
├── ca.go
├── certificate.go
├── cache.go
├── tls.go
├── upstream.go
├── router.go
├── http1.go
├── http2.go
├── websocket.go
└── rewrite/
    ├── engine.go
    ├── rule.go
    ├── matcher.go
    └── body.go
```

配置：

```text
option/
└── mitm.go
```

平台：

```text
platform/apple/jailbreak/
├── ca/
├── launcher/
└── trust/
```

测试：

```text
test/
├── mitm_config_test.go
├── mitm_cert_test.go
├── mitm_match_test.go
├── mitm_http_test.go
└── mitm_router_test.go
```

---

# 45. 开发顺序

不要一次把所有代码写进去。

建议严格按照：

```text
Phase 1
│
├── option
├── constant
├── registry
└── config validation
```

↓

```text
Phase 2
│
├── CA
├── certificate
├── certificate cache
└── SNI parser
```

↓

```text
Phase 3
│
├── TUN interception interface
├── TLS server
└── TLS upstream
```

↓

```text
Phase 4
│
├── HTTP/1.1
├── HTTP/2
└── WebSocket
```

↓

```text
Phase 5
│
├── Router integration
└── Outbound integration
```

↓

```text
Phase 6
│
├── Rewrite
└── compression
```

↓

```text
Phase 7
│
├── iOS jailbreak CA
├── LaunchDaemon
└── debug trust
```

↓

```text
Phase 8
│
├── integration tests
├── performance
└── production hardening
```

---

# 46. 第一阶段完成标准

第一阶段不要碰 HTTP。

只要求：

```text
sing-box
   │
   ▼
parse config
   │
   ▼
services[type=mitm]
   │
   ▼
MITM Service
   │
   ▼
load CA
   │
   ▼
Service Manager
```

能够成功：

```bash
sing-box check -c config.json
```

并通过：

```text
go test ./...
```

---

# 47. 第二阶段完成标准

能够：

```text
TCP
 ↓
TLS ClientHello
 ↓
SNI example.com
 ↓
dynamic certificate
 ↓
TLS handshake
```

也就是说，用测试客户端连接：

```text
https://example.com
```

能够在 MITM 侧看到：

```text
SNI=example.com
```

---

# 48. 第三阶段完成标准

真正完成：

```text
iOS
 ↓
HTTPS
 ↓
TUN
 ↓
MITM
 ↓
TLS
 ↓
HTTP
 ↓
Router
 ↓
Outbound
 ↓
Server
```

此时才开始加入 rewrite。

---

# 49. 第四阶段完成标准

支持：

```text
HTTP/1.1
HTTP/2
WebSocket
```

并且：

```text
Router
```

对三种协议产生统一的 outbound routing。

---

# 50. 最终数据流

最终系统应该达到：

```text
┌──────────────────────────────────────────────┐
│                    iOS                       │
│                                              │
│  ┌─────────────┐                             │
│  │ Application │                             │
│  └──────┬──────┘                             │
│         │ HTTPS                               │
│         ▼                                    │
│  ┌─────────────┐                             │
│  │     TUN     │                             │
│  └──────┬──────┘                             │
│         │ TCP                                │
│         ▼                                    │
│  ┌─────────────────────┐                     │
│  │   MITM Interceptor  │                     │
│  └──────────┬──────────┘                     │
│             │                                │
│        TLS ClientHello                       │
│             │                                │
│             ▼                                │
│      ┌──────────────┐                        │
│      │ SNI Matcher  │                        │
│      └──────┬───────┘                        │
│             │                                │
│             ▼                                │
│      ┌──────────────┐                        │
│      │ Certificate  │                        │
│      │   Engine     │                        │
│      └──────┬───────┘                        │
│             │                                │
│             ▼                                │
│      ┌──────────────┐                        │
│      │ TLS Server   │                        │
│      └──────┬───────┘                        │
│             │                                │
│       decrypted HTTP                         │
│             │                                │
│      ┌──────▼───────┐                        │
│      │ HTTP Engine   │                        │
│      │ H1 / H2 / WS  │                        │
│      └──────┬───────┘                        │
│             │                                │
│      ┌──────▼───────┐                        │
│      │ Rewrite       │                        │
│      └──────┬───────┘                        │
│             │                                │
│      ┌──────▼───────┐                        │
│      │ sing-box      │                        │
│      │ Router        │                        │
│      └──────┬───────┘                        │
│             │                                │
│      ┌──────▼───────┐                        │
│      │ Outbound      │                        │
│      └──────┬───────┘                        │
│             │                                │
└─────────────┼────────────────────────────────┘
              │
              ▼
          Internet
```

---

# 51. 最终配置职责

最终整个配置体系保持非常清晰：

```text
inbounds
    ↓
流量从哪里进

services.mitm
    ↓
哪些 HTTPS 流量需要解密

rewrite
    ↓
HTTP 数据如何修改

route
    ↓
请求应该走哪个 outbound

outbounds
    ↓
如何访问真实服务器
```

即：

```text
Inbound
   ↓
MITM
   ↓
HTTP
   ↓
Route
   ↓
Outbound
```

而不是让 MITM 变成一个第二版 sing-box。

---

# 52. 当前实现状态说明

前面讨论中的部分代码是**架构骨架**，不是已经针对某个具体 `testing` commit 验证过的可编译 patch。

目前可以确定的核心方向是：

```text
Service Registry
        +
MITM Service
        +
现有 Router
        +
TUN interception interface
        +
dynamic certificate
        +
HTTP/1.1 / HTTP/2 / WebSocket
```

尤其是 `testing` 分支在持续变化，因此真正制作：

```text
git apply
```

级别 patch 时，必须绑定一个具体 commit。

这样才能避免：

```text
undefined: service.FromContext
undefined: adapter.Service
undefined: option.Options
```

这类由于分支 API 变化造成的问题。

---

## 53. 推荐的最终 Git 提交历史

实际开发时可以拆成：

```text
commit 1
mitm: add service options

commit 2
mitm: register service

commit 3
mitm: add CA loader

commit 4
mitm: add certificate issuer

commit 5
mitm: add TLS interception

commit 6
mitm: integrate TUN interceptor

commit 7
mitm: integrate router

commit 8
mitm: add HTTP/1.1

commit 9
mitm: add HTTP/2

commit 10
mitm: add websocket

commit 11
mitm: add rewrite engine

commit 12
mitm: add iOS jailbreak integration

commit 13
mitm: add integration tests
```

这样任何阶段出问题都可以：

```bash
git revert <commit>
```

而不用回滚整个 MITM 系统。

---
