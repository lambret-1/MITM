# MITM 模块开发进度

基于 sing-box `testing` 分支（commit `c6c74c9`），按 README 第 45 节开发顺序推进。

**源码位置：** MITM 仓库 `sing-box` 分支，MITM 模块位于 `protocol/mitm/`。

## 目录结构（按 README 第 44 节）

```
protocol/mitm/
├── service.go        # 服务主体、注册、生命周期
├── ca.go             # CA 加载与验证
├── certificate.go    # 动态叶子证书签发
├── cache.go          # 证书 LRU 缓存
├── matcher.go        # 域名匹配器
├── clienthello.go    # TLS ClientHello 解析（SNI/ALPN）
├── interceptor.go    # TUN 拦截主流程（已实现）
├── tls.go            # TLS 终止（已实现）
├── upstream.go       # 上游 TLS 连接（已实现）
├── router.go         # Router 集成（骨架）
├── http1.go          # HTTP/1.1 引擎（已实现）
├── http2.go          # HTTP/2 引擎（已实现）
├── websocket.go      # WebSocket 处理（已实现）
├── service_test.go   # 单元测试（13 个）
└── rewrite/
    ├── engine.go     # 重写引擎（骨架）
    ├── rule.go       # 重写规则
    ├── matcher.go    # 重写匹配器
    └── body.go       # Body 重写（含 gzip 解压/压缩）
```

## Phase 1：配置结构 + 服务注册 + CA 加载 ✅

**完成标准（README 第 46 节）：**
- `sing-box check -c config.json` 能正确解析 `services[type=mitm]`
- MITM Service 能加载并验证 CA 证书
- 通过 `go test ./...`

**已实现：**
- `option/mitm.go`：`MITMServiceOptions` / `MITMCAOptions` / `MITMMatchOptions` / `MITMRewriteOptions`
- `constant/proxy.go`：新增 `TypeMITM = "mitm"`
- `protocol/mitm/service.go`：服务注册、构造、生命周期
- `protocol/mitm/ca.go`：CA 加载验证（IsCA + KeyUsageCertSign）
- `include/mitm.go` + `include/registry.go`：服务注册接入

## Phase 2：动态证书 + SNI 解析 ✅

**完成标准（README 第 47 节）：**
- TCP → TLS ClientHello → SNI 提取 → 动态证书 → TLS 握手

**已实现：**
- `protocol/mitm/certificate.go`：ECDSA P256 动态签发叶子证书，含缓存
- `protocol/mitm/cache.go`：LRU 证书缓存（默认容量 4096）
- `protocol/mitm/matcher.go`：域名匹配器（精确 + 后缀）
- `protocol/mitm/clienthello.go`：TLS ClientHello 解析，提取 SNI 和 ALPN，含回退读取器

## Phase 3：TUN 拦截 + TLS 终止 + 上游连接 ✅

**完成标准（README 第 48 节）：**
- TUN 捕获的 HTTPS 流量能被 MITM 拦截
- SNI 匹配命中后完成 TLS 终止
- 通过 Router 建立上游连接并完成上游 TLS
- 双向转发明文流量

**已实现：**
- `protocol/mitm/interceptor.go`：完整拦截流程
  - 读取 ClientHello → SNI 提取 → 域名匹配
  - 不匹配：回退读取器包装连接，交回 Router 正常路由
  - 匹配：签发叶子证书 + tls.Server 终止客户端 TLS
  - 通过 net.Pipe + Router 建立上游连接 + tls.Client 上游 TLS
  - 双向转发明文（Phase 4 将替换为 HTTP 引擎）
- `protocol/mitm/clienthello.go`：回退读取器实现完整 net.Conn 接口
- `protocol/tun/inbound.go`：`NewConnectionEx` 和 `autoRedirectHandler.NewConnectionEx` 中接入 MITM 拦截器
- 跨包导出方法：`ShouldIntercept(metadata)` / `Intercept(ctx, conn, metadata, router, onClose)`

## Phase 4：HTTP/1.1 + HTTP/2 + WebSocket 引擎 ✅

**完成标准（README 第 49 节）：**
- 支持 HTTP/1.1、HTTP/2、WebSocket 三种协议
- 根据 ALPN 协商自动选择引擎
- 三种协议统一通过 Router 进行 outbound routing

**已实现：**
- `protocol/mitm/http1.go`：完整 HTTP/1.1 引擎
  - bufio 循环读取请求，支持 Keep-Alive 持久连接
  - 从 Host 头提取域名，通过 Router 建立上游 TLS 连接
  - 请求转发 → 响应读取 → 写回客户端
  - 检测 WebSocket Upgrade，切换到双向流转发
  - 处理 Content-Length、Transfer-Encoding、chunked、trailers
- `protocol/mitm/http2.go`：基于 golang.org/x/net/http2 的多路复用引擎
  - http2.Server.ServeConn 管理连接和流生命周期
  - 每个流独立通过 Router 建立上游连接
  - http2.Transport 发送上游请求，流式返回响应
  - 不自行实现 framing，完全依赖标准库扩展
- `protocol/mitm/websocket.go`：WebSocket Upgrade 检测工具
- `protocol/mitm/upstream.go`：实现 建立上游连接（net.Pipe + Router + TLS）
- `protocol/mitm/interceptor.go`：ALPN 协商选择引擎（h2 → HTTP/2，其他 → HTTP/1.1）

## Phase 5-8：骨架已就位，待实现

| 阶段 | 内容 | 状态 |
| --- | --- | --- |
| Phase 5 | Router 集成深化 | 骨架（router.go，已适配 RouteConnection API） |
| Phase 6 | Rewrite 引擎 | 骨架（rewrite/，body.go 已含 gzip 解压压缩） |
| Phase 7 | iOS 越狱层 | 待开始 |
| Phase 8 | 集成测试 + 性能硬化 | 待开始 |

## 验证结果

- `go build ./cmd/sing-box` ✅ 编译通过（53MB）
- `go build $(go list ./... | grep -v '/experimental/')` ✅ 全部包编译通过
- `sing-box check -c` 含 mitm 服务的配置 ✅ 校验通过
- `go test ./protocol/mitm/...` ✅ 13/13 通过
- GitHub Actions CI（mitm-ci.yml）✅ 全部 12 步骤 success
  - 编译主程序、编译全部包、MITM 单元测试、相关模块测试、CA 生成、配置校验、产物上传

## 配置格式

注意：sing-box 的 service 配置是**扁平结构**，`Options` 字段 tag 为 `json:"-"`，通过 `badjson.UnmarshallExcludedContext` 解析。

```json
{
  "services": [
    {
      "type": "mitm",
      "tag": "mitm",
      "enabled": true,
      "ca": {
        "certificate": "/path/to/ca.pem",
        "private_key": "/path/to/ca.key"
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
