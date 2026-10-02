# MITM 模块开发进度

基于 sing-box `testing` 分支（commit `c6c74c9`），按 README 第 45 节开发顺序推进。

## Phase 1：配置结构 + 服务注册 + CA 加载 ✅

**完成标准（README 第 46 节）：**
- `sing-box check -c config.json` 能正确解析 `services[type=mitm]`
- MITM Service 能加载并验证 CA 证书
- 通过 `go test ./...`

**已实现文件：**

| 文件 | 说明 |
| --- | --- |
| `option/mitm.go` | `MITMServiceOptions` / `MITMCAOptions` / `MITMMatchOptions` / `MITMRewriteOptions` 配置结构 |
| `constant/proxy.go` | 新增 `TypeMITM = "mitm"` 常量 |
| `service/mitm/service.go` | MITM Service 主体：注册、构造、CA 加载验证、生命周期 |
| `service/mitm/service_test.go` | 8 个单元测试，覆盖启用/未启用、缺配置、合法 CA、非 CA 拒绝、生命周期、注册、上下文获取 |
| `include/mitm.go` | 服务注册入口 |
| `include/registry.go` | 在 `ServiceRegistry()` 中接入 `registerMITMService` |

**配置格式（注意：扁平结构，非嵌套 options）：**
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

> README 第 4.1 节示例中使用了嵌套 `"options"` 字段，这与 sing-box 实际的
> `Service` 反序列化机制不符（`Options` 字段 tag 为 `json:"-"`，通过
> `badjson.UnmarshallExcludedContext` 扁平解析）。实际配置应为上述扁平格式。

**验证结果：**
- `go build ./cmd/sing-box` ✅ 编译通过
- `sing-box check -c` 含 mitm 服务的配置 ✅ 校验通过
- `go test ./service/mitm/...` ✅ 8/8 通过
- `go test ./option/... ./constant/... ./include/...` ✅ 无回归

**CA 验证逻辑：**
1. 读取 PEM 证书与私钥文件
2. `tls.X509KeyPair` 解析证书对
3. `x509.ParseCertificate` 解析证书实体
4. 校验 `IsCA == true`
5. 校验 `KeyUsage` 包含 `KeyUsageCertSign`

**Patch 文件：** `phase1-mitm-service-options.patch`（16KB，可直接 `git apply` 到 sing-box testing 分支）

## Phase 2：动态证书 + SNI 解析（待开始）

- 证书签发引擎（基于根证书动态签发叶子证书）
- 证书 LRU 缓存（容量 4096，带 TTL）
- TLS ClientHello 解析与 SNI 提取
- 域名匹配器（domain / domain_suffix）

## Phase 3：TUN 拦截 + TLS 终止（待开始）

## Phase 4：HTTP/1.1 + HTTP/2 + WebSocket（待开始）

## Phase 5：Router 集成（待开始）

## Phase 6：Rewrite 引擎（待开始）

## Phase 7：iOS 越狱层（待开始）

## Phase 8：集成测试 + 性能硬化（待开始）
