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
├── interceptor.go    # TUN 拦截接口（骨架）
├── tls.go            # TLS 终止（骨架）
├── upstream.go       # 上游连接（骨架）
├── router.go         # Router 集成（骨架）
├── http1.go          # HTTP/1.1 引擎（骨架）
├── http2.go          # HTTP/2 引擎（骨架）
├── websocket.go      # WebSocket 处理（骨架）
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

## Phase 3-8：骨架已就位，待实现

| 阶段 | 内容 | 状态 |
| --- | --- | --- |
| Phase 3 | TUN 拦截 + TLS 终止 + 上游连接 | 骨架（interceptor.go / tls.go / upstream.go） |
| Phase 4 | HTTP/1.1 + HTTP/2 + WebSocket | 骨架（http1.go / http2.go / websocket.go） |
| Phase 5 | Router 集成 | 骨架（router.go，已适配 RouteConnection API） |
| Phase 6 | Rewrite 引擎 | 骨架（rewrite/，body.go 已含 gzip 解压压缩） |
| Phase 7 | iOS 越狱层 | 待开始 |
| Phase 8 | 集成测试 + 性能硬化 | 待开始 |

## 验证结果

- `go build ./cmd/sing-box` ✅ 编译通过
- `sing-box check -c` 含 mitm 服务的配置 ✅ 校验通过
- `go test ./protocol/mitm/...` ✅ 13/13 通过
  - 服务构造（启用/未启用/缺配置/合法CA/非CA拒绝）
  - 生命周期、注册、上下文获取
  - 域名匹配器（精确/后缀）
  - 证书缓存 LRU 淘汰
  - 动态叶子证书签发 + 缓存命中
  - TLS ClientHello SNI/ALPN 解析

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
