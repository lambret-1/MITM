# sing-box MITM 内核接入指南

> 本文档说明 MITM 模块如何接入 sing-box 内核，包括目录结构、配置注册、服务注册、TUN 拦截接入、Router 集成等关键技术点。适用于基于 sing-box `testing` 分支的二次开发。

## 1. 模块定位

MITM 模块作为 sing-box 的 **service**（服务）存在，与 `server`、`observer` 等服务平级。它不直接处理入站流量，而是通过 **TUN 入站拦截接口** 介入 HTTPS 流量，完成 TLS 终止后将解密后的 HTTP 流量交回 sing-box Router 进行分流。

```text
sing-box 内核
├── inbounds（入站）
│   └── tun ← MITM 拦截接入点
├── services（服务）
│   └── mitm ← 本模块
├── router（路由）
│   └── MITM 解密后流量经此分流
└── outbounds（出站）
    └── MITM 上游连接经此建立
```

## 2. 目录结构

```text
MITM 仓库根目录（sing-box 分支）
├── option/
│   └── mitm.go              # 配置结构定义
├── constant/
│   └── proxy.go             # 新增 TypeMITM 常量
├── include/
│   ├── mitm.go              # 服务注册入口
│   └── registry.go          # 注册表（调用 RegisterService）
├── protocol/
│   ├── mitm/                # MITM 核心模块
│   │   ├── service.go       # 服务主体、注册、生命周期
│   │   ├── ca.go            # CA 加载与验证
│   │   ├── certificate.go   # 动态叶子证书签发
│   │   ├── cache.go         # 证书 LRU 缓存
│   │   ├── matcher.go       # 域名匹配器
│   │   ├── clienthello.go   # TLS ClientHello 解析 + 回退读取器
│   │   ├── interceptor.go   # 拦截主流程（导出 ShouldIntercept/Intercept）
│   │   ├── tls.go           # TLS 终止
│   │   ├── upstream.go      # 上游连接（Router + net.Pipe + TLS）
│   │   ├── router.go        # Router 集成工具
│   │   ├── http1.go         # HTTP/1.1 引擎
│   │   ├── http2.go         # HTTP/2 引擎（golang.org/x/net/http2）
│   │   ├── websocket.go     # WebSocket Upgrade 检测
│   │   ├── service_test.go  # 单元测试（13 个）
│   │   ├── integration_test.go # 集成测试（6 个）
│   │   └── rewrite/         # 重写引擎子包
│   │       ├── engine.go    # 重写引擎主体（导出 Engine/NewEngine）
│   │       ├── rule.go      # 规则定义与匹配
│   │       ├── matcher.go   # 匹配工具函数
│   │       └── body.go      # Body 重写（gzip 解压/重压缩）
│   └── tun/
│       └── inbound.go       # TUN 入站（已修改，接入 MITM 拦截器）
├── cmd/sing-box/
│   └── cmd_generate_mitm_ca.go # CLI: sing-box generate mitm-ca
└── deploy/ios/              # iOS 越狱部署
    ├── build-ios.sh         # 交叉编译脚本
    ├── install.sh           # 一键安装脚本
    ├── config/config.json   # 部署配置模板
    ├── com.sing-box.mitm.plist # LaunchDaemon
    └── README-实测指南.md    # 实测指南
```

## 3. 配置接入（option/mitm.go）

### 3.1 配置结构

sing-box 的 service 配置是**扁平结构**，`Options` 字段 tag 为 `json:"-"`，通过 `badjson.UnmarshallExcludedContext` 解析。**不是** README 示例中的嵌套 `"options"` 字段。

```go
type MITMServiceOptions struct {
    Enabled         bool                `json:"enabled,omitempty"`
    CA              MITMCAOptions       `json:"ca,omitempty"`
    Match           MITMMatchOptions    `json:"match,omitempty"`
    Rewrite         MITMRewriteOptions  `json:"rewrite,omitempty"`
    OnError         string              `json:"on_error,omitempty"`         // bypass / block
    UpstreamTimeout int                 `json:"upstream_timeout,omitempty"` // 默认 30 秒
}
```

### 3.2 配置示例

```json
{
  "services": [
    {
      "type": "mitm",
      "tag": "mitm",
      "enabled": true,
      "ca": {
        "certificate": "/etc/sing-box/ca.pem",
        "private_key": "/etc/sing-box/ca.key"
      },
      "match": {
        "domain_suffix": ["example.com"]
      },
      "rewrite": {
        "enabled": true,
        "max_body_size": 10485760,
        "rules": [
          {
            "domain_suffix": ["example.com"],
            "response_header": {"X-MITM": "sing-box"},
            "body_replace": [{"find": "old", "replace": "new"}]
          }
        ]
      },
      "on_error": "bypass",
      "upstream_timeout": 30
    }
  ]
}
```

## 4. 常量注册（constant/proxy.go）

在 `constant/proxy.go` 中新增 MITM 类型标识：

```go
const (
    TypeMITM = "mitm"
)
```

此常量用于：
- 服务注册时的类型标识
- `InboundContext.InboundType` 标记
- route 规则中按入站类型匹配

## 5. 服务注册（include/）

### 5.1 include/mitm.go

```go
package include

import "github.com/sagernet/sing-box/protocol/mitm"

func init() {
    // 注册到 include 注册表，由 registry.go 统一调用
}
```

### 5.2 include/registry.go

在 `Registry()` 函数中调用 MITM 服务注册：

```go
mitm.RegisterService(registry)
```

### 5.3 protocol/mitm/service.go

```go
func RegisterService(registry *boxService.Registry) {
    boxService.Register[option.MITMServiceOptions](registry, C.TypeMITM, NewService)
}
```

`NewService` 签名必须符合 `boxService.Register` 的要求：
```go
func NewService(ctx context.Context, logger log.ContextLogger, tag string, options option.MITMServiceOptions) (adapter.Service, error)
```

## 6. 服务生命周期

### 6.1 构造阶段（NewService）

1. 校验配置完整性（CA 证书/私钥路径）
2. 加载并验证 CA 证书（必须 `IsCA=true` 且 `KeyUsage` 包含 `CertSign`）
3. 初始化域名匹配器
4. 初始化重写引擎
5. 初始化证书 LRU 缓存（容量 4096）
6. 将自身注册到 context：`service.MustRegister[*Service](ctx, svc)`

### 6.2 启动阶段（Start）

```go
func (s *Service) Start(stage adapter.StartStage, scope *adapter.Scope) error {
    if stage != adapter.StartStateStart {
        return nil
    }
    // MITM 拦截由 TUN 入站主动调用，无需后台 goroutine
    return nil
}
```

### 6.3 上下文获取

TUN 入站通过 context 获取 MITM 服务实例：
```go
mitmService := mitm.FromContext(ctx)
```

## 7. TUN 拦截接入（protocol/tun/inbound.go）

### 7.1 接入点

在 `Inbound.NewConnectionEx` 和 `autoRedirectHandler.NewConnectionEx` 中，调用 `router.RouteConnectionEx` **之前**接入 MITM 拦截：

```go
// MITM 拦截：判断是否需要 HTTPS 解密
mitm服务 := mitm.FromContext(ctx)
if mitm服务 != nil && mitm服务.ShouldIntercept(metadata) {
    mitm服务.Intercept(ctx, conn, metadata, t.router, onClose)
    return
}
t.router.RouteConnectionEx(ctx, conn, metadata, onClose)
```

### 7.2 导出接口

由于 Go 的中文标识符无法跨包导出，TUN 接入使用英文导出名：

| 导出名 | 中文含义 | 签名 |
| --- | --- | --- |
| `ShouldIntercept` | 是否拦截 | `func(metadata adapter.InboundContext) bool` |
| `Intercept` | 拦截 | `func(ctx, conn, metadata, router, onClose)` |
| `FromContext` | 从上下文获取 | `func(ctx) *Service` |

### 7.3 拦截判断逻辑

```go
func (s *Service) ShouldIntercept(metadata adapter.InboundContext) bool {
    if !s.是否启用() { return false }
    if s.匹配器 == nil || s.匹配器.是否为空() { return false }
    if metadata.Protocol != "" { return false } // 跳过 DNS 等特殊流量
    return metadata.Destination.Port == 443       // 仅 HTTPS 端口
}
```

## 8. 拦截主流程（protocol/mitm/interceptor.go）

```text
TCP 连接（来自 TUN）
    │
    ▼
读取 TLS ClientHello → 提取 SNI / ALPN
    │
    ├── 读取失败 → 关闭连接
    │
    ├── SNI 不匹配 → 回退读取器包装 → Router 正常路由
    │
    └── SNI 匹配
            │
            ▼
    动态签发叶子证书（ECDSA P-256，LRU 缓存）
            │
            ▼
    tls.Server 终止客户端 TLS
            │
            ▼
    ALPN 协商结果
    ├── h2 → HTTP/2 引擎（http2.Server.ServeConn）
    └── 其他 → HTTP/1.1 引擎（bufio 循环读取）
            │
            ▼
    每个请求：建立上游连接 → 发送请求 → 读取响应 → 重写 → 返回客户端
```

### 8.1 回退读取器

`clienthello.go` 中的 `回退读取器` 实现完整 `net.Conn` 接口，将已读取的 ClientHello 数据放回流头部，使不匹配的连接能无缝交回 Router 正常路由。

## 9. Router 集成（protocol/mitm/upstream.go）

### 9.1 核心原则

**MITM 不自行选择 Proxy，所有出站流量必须经过 sing-box Router 决策。**

### 9.2 上游连接建立

使用 `net.Pipe` 创建管道对，一端交给 Router，另一端作为上游 TCP 连接：

```go
上游客户端, 上游服务端 := net.Pipe()
go 路由器.RouteConnectionEx(ctx, 上游服务端, 上游元数据, nil)
// 在 上游客户端 上做 TLS 握手
```

### 9.3 上游 metadata

```go
上游元数据 := 元数据
上游元数据.Inbound = "mitm"           // 标记来源，便于 route 规则区分
上游元数据.InboundType = "mitm"
上游元数据.Destination = M.ParseSocksaddrHostPort(域名, 443) // 域名而非 IP
上游元数据.Network = "tcp"
```

**关键：** Destination 设置为域名而非原始 IP，使 Router 能按 `domain`、`domain_suffix`、`geosite` 等规则匹配。

### 9.4 上游 TLS 超时

```go
握手上下文, 取消 := context.WithTimeout(ctx, time.Duration(s.获取上游超时())*time.Second)
// goroutine 中执行 Handshake，select 等待完成或超时
```

## 10. HTTP 引擎

### 10.1 HTTP/1.1（protocol/mitm/http1.go）

- `bufio.Reader` 循环读取请求，支持 Keep-Alive
- 每个请求独立建立上游连接（后续可优化连接池）
- 检测 `Connection: Upgrade` + `Upgrade: websocket` → 切换双向转发
- 请求前 `RewriteRequest`，响应后 `RewriteResponse`

### 10.2 HTTP/2（protocol/mitm/http2.go）

- 使用 `golang.org/x/net/http2`，**不自行实现 framing**
- `http2.Server.ServeConn` 管理连接和流生命周期
- 每个流通过自定义 `http.Handler` 处理
- `http2.Transport` + 自定义 `DialTLSContext` 发送上游请求

### 10.3 WebSocket（protocol/mitm/websocket.go）

- 在 HTTP/1.1 引擎中检测 Upgrade 请求
- 转发握手请求/响应，101 后转为原始双向流转发
- 排空 bufio 中已缓冲的数据后开始 io.Copy

## 11. 重写引擎（protocol/mitm/rewrite/）

### 11.1 导出类型

| 导出名 | 中文含义 |
| --- | --- |
| `Engine` | 重写引擎 |
| `NewEngine` | 创建引擎 |
| `RewriteRequest` | 重写请求 |
| `RewriteResponse` | 重写响应 |
| `IsEnabled` | 是否启用 |

### 11.2 Body 重写流程

```text
响应体（可能 gzip 压缩）
    │
    ▼
LimitReader 读取（max_body_size 限制）
    │
    ├── 超过限制 → 跳过重写，原样返回
    │
    └── 未超过
            │
            ▼
        解压（gzip / identity）
            │
            ▼
        bytes.ReplaceAll 应用所有替换规则
            │
            ▼
        重压缩（按原 Content-Encoding）
            │
            ▼
        更新 Content-Length / Content-Encoding / Transfer-Encoding
```

## 12. CLI 工具（cmd/sing-box/cmd_generate_mitm_ca.go）

```bash
sing-box generate mitm-ca \
    --name "sing-box MITM CA" \
    --validity 3650 \
    --output ./mitm
```

输出：
- `ca.pem`（0644）— 根证书，安装到 iOS 系统信任存储
- `ca.key`（0600）— 私钥，放在 sing-box 配置目录

使用 ECDSA P-256 算法，证书属性：`IsCA=true`、`KeyUsage=CertSign|DigitalSignature`、`MaxPathLen=0`。

## 13. 编译验证

### 13.1 本地编译

```bash
# 编译主程序
go build -o sing-box ./cmd/sing-box

# 编译全部包（排除 experimental 平台限制包）
go build $(go list ./... | grep -v '/experimental/')

# 运行测试
go test ./protocol/mitm/... ./option/...

# 配置校验
sing-box check -c config.json

# 生成 CA
sing-box generate mitm-ca --output /tmp/mitm-test
```

### 13.2 iOS 交叉编译

```bash
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 \
    go build -o sing-box-ios -trimpath -ldflags="-s -w" ./cmd/sing-box
```

> **注意：** `GOOS=ios` 需要 CGO + iOS SDK，Linux 环境无法直接构建。使用 `darwin/arm64` 替代，可在 iOS 越狱设备上运行（可能需要 `ldid -S` 伪签名）。

### 13.3 CI/CD

`.github/workflows/mitm-ci.yml` 仅在 `sing-box` 分支触发，包含：
1. Go 1.25.5 环境设置
2. 编译主程序
3. 编译全部包（排除 experimental）
4. MITM 模块单元测试
5. 相关模块测试
6. 生成测试 CA 证书
7. MITM 配置校验
8. 未启用 MITM 配置校验
9. 构建产物上传

## 14. 关键设计决策

| 决策 | 原因 |
| --- | --- |
| service 而非 inbound | MITM 不监听端口，由 TUN 主动调用 |
| 扁平配置结构 | sing-box service 框架要求，`Options` 字段 `json:"-"` |
| 英文导出方法名 | Go 中文标识符无法跨包导出，加中文注释 |
| net.Pipe + Router 建立上游 | 不绕过 Router，所有流量经 route 规则分流 |
| Destination 设为域名 | Router 按域名规则匹配，而非原始 IP |
| golang.org/x/net/http2 | 不自行实现 HTTP/2 framing |
| ECDSA P-256 叶子证书 | 性能优于 RSA，移动端友好 |
| LRU 证书缓存容量 4096 | 避免长期运行内存膨胀 |
| Body 大小限制默认 10MB | 防止 io.ReadAll 内存溢出 |
| Inbound 标记为 "mitm" | route 规则可区分 MITM 解密后流量 |

## 15. 已验证死路（避免重蹈覆辙）

1. **浅克隆直接 push 到 GitHub 必失败**（缺 parent object）→ 必须 `git archive HEAD | tar -x` 导出到独立仓库再 force push
2. **force push 不触发 GitHub Actions** → 需额外空提交正常推送，或用 `workflow_dispatch` API 手动触发
3. **experimental/libbox 在 Linux 下有 oomprofile 链接错误** → CI 中 `go build` 需排除 `/experimental/`
4. **Go 中文标识符无法跨包导出** → 跨包 API 用英文导出名 + 中文注释
5. **sing-box service 配置是扁平结构** → 不是嵌套 `"options"` 字段
6. **GOOS=ios 需要 CGO + iOS SDK** → Linux 上用 darwin/arm64 替代
7. **TLS 终止失败后无法回退到正常路由** → 已发送 ServerHello 等数据，只能关闭连接

## 16. 扩展指南

### 16.1 添加新的匹配条件

1. 在 `option/mitm.go` 的 `MITMMatchOptions` 添加字段
2. 在 `protocol/mitm/matcher.go` 的 `域名匹配器` 添加匹配逻辑
3. 在 `ShouldIntercept` 或 `Intercept` 中调用

### 16.2 添加新的重写规则类型

1. 在 `option/mitm.go` 的 `MITMRewriteRule` 添加字段
2. 在 `protocol/mitm/rewrite/rule.go` 的 `规则` 结构体添加字段
3. 在 `rewrite/engine.go` 的 `NewEngine` 中从配置加载
4. 在 `RewriteRequest` / `RewriteResponse` 中应用

### 16.3 支持新的入站类型（如 mixed）

在对应入站的 `NewConnectionEx` 中，参照 TUN 的接入方式添加：
```go
mitm服务 := mitm.FromContext(ctx)
if mitm服务 != nil && mitm服务.ShouldIntercept(metadata) {
    mitm服务.Intercept(ctx, conn, metadata, router, onClose)
    return
}
```

### 16.4 连接池优化

当前每个 HTTP 请求独立建立上游连接。可在 `Service` 中添加连接池：
- key：域名 + outbound tag
- 使用 `sync.Pool` 或自定义 LRU 池
- HTTP/1.1 复用 keep-alive 连接
- HTTP/2 天然多路复用，可共享一个上游连接

## 17. 相关文件索引

| 文件 | 职责 |
| --- | --- |
| `option/mitm.go` | 配置结构（Service/CA/Match/Rewrite/Rule/BodyReplace） |
| `constant/proxy.go` | TypeMITM 常量 |
| `include/mitm.go` + `registry.go` | 服务注册 |
| `protocol/mitm/service.go` | 服务主体、生命周期、上下文注册 |
| `protocol/mitm/ca.go` | CA 加载验证 |
| `protocol/mitm/certificate.go` | 动态叶子证书签发 |
| `protocol/mitm/cache.go` | LRU 证书缓存 |
| `protocol/mitm/matcher.go` | 域名匹配器 |
| `protocol/mitm/clienthello.go` | ClientHello 解析 + 回退读取器 |
| `protocol/mitm/interceptor.go` | 拦截主流程（ShouldIntercept/Intercept） |
| `protocol/mitm/tls.go` | TLS 终止 |
| `protocol/mitm/upstream.go` | 上游连接（Router + net.Pipe + TLS 超时） |
| `protocol/mitm/router.go` | Router 集成工具 |
| `protocol/mitm/http1.go` | HTTP/1.1 引擎 |
| `protocol/mitm/http2.go` | HTTP/2 引擎 |
| `protocol/mitm/websocket.go` | WebSocket 检测 |
| `protocol/mitm/rewrite/engine.go` | 重写引擎（Engine/NewEngine/RewriteRequest/RewriteResponse） |
| `protocol/mitm/rewrite/rule.go` | 规则定义与匹配 |
| `protocol/mitm/rewrite/body.go` | Body 重写（gzip） |
| `protocol/tun/inbound.go` | TUN 接入点（第 642-645、692-695 行） |
| `cmd/sing-box/cmd_generate_mitm_ca.go` | CLI: generate mitm-ca |
| `deploy/ios/` | iOS 越狱部署包 |
| `.github/workflows/mitm-ci.yml` | CI/CD 流水线 |
