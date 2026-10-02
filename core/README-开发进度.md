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
├── router.go         # Router 集成（已实现）
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

## Phase 5：Router 集成深化 + Outbound 集成 ✅

**完成标准（README 第 23、24 节）：**
- 解密后的 HTTP 请求构造 sing-box metadata（domain = Host）
- 所有出站流量经过 sing-box Router 决策（domain/geosite/geoip/rule-set）
- MITM 不自行选择 Proxy，全部由 Router 处理
- Inbound 标记为 "mitm"，便于 route 规则区分

**已实现：**
- `protocol/mitm/upstream.go`：建立上游连接时构造域名 metadata
  - Destination = 域名:443（Router 按域名规则路由）
  - Inbound/InboundType = "mitm"（route 规则可区分 MITM 流量）
  - net.Pipe + Router.RouteConnectionEx 建立出站连接
- `protocol/mitm/router.go`：完善 构造入站上下文 和 路由连接，使用 TypeMITM 常量
- HTTP/1.1 和 HTTP/2 引擎每个请求独立通过 Router 建立上游连接

## Phase 6：Rewrite 引擎 + 压缩处理 ✅

**完成标准（README 第 21、22 节）：**
- 支持请求头/响应头修改（设置/删除）
- 支持响应体替换（含 gzip 解压/重压缩）
- 正确更新 Content-Length、Content-Encoding、Transfer-Encoding
- 超过 max_body_size 的响应跳过重写

**已实现：**
- `option/mitm.go`：扩展 MITMRewriteOptions
  - `max_body_size`：最大重写体大小（默认 10MB）
  - `rules[]`：重写规则列表（domain_suffix/path_prefix/method/request_header/response_header/body_replace）
- `rewrite/engine.go`：重写引擎主体（导出类型 Engine/NewEngine/RewriteRequest/RewriteResponse）
  - 从配置加载规则，按顺序匹配并应用
  - 响应重写收集所有命中规则的 Body 替换，调用体重写器
- `rewrite/body.go`：体重写器
  - gzip 解压 → 字节替换 → gzip 重压缩
  - 带大小限制（LimitReader），超过则跳过
  - 正确更新 Content-Length、Content-Encoding、Transfer-Encoding
- `rewrite/rule.go`：规则匹配（域名后缀/路径前缀/方法）+ 头修改
- `service.go`：初始化重写引擎，提供 获取重写引擎 方法
- `http1.go` / `http2.go`：请求前调用 RewriteRequest，响应后调用 RewriteResponse

## Phase 7：iOS 越狱层 ✅

**完成标准（README 第 29-32 节）：**
- 提供 MITM CA 证书生成 CLI 工具
- 提供 LaunchDaemon 开机自启配置
- 提供 iOS 部署完整指南（CA 安装/配置/调试信任/故障排查）

**已实现：**
- `cmd/sing-box/cmd_generate_mitm_ca.go`：`sing-box generate mitm-ca` 命令
  - ECDSA P-256 算法，CA:TRUE，KeyUsage: CertSign
  - 参数：--name（证书名）、--validity（有效期天）、--output（输出目录）
  - 输出 ca.pem（0644）和 ca.key（0600）
- `deploy/ios/com.sing-box.mitm.plist`：LaunchDaemon 配置模板
  - RunAtLoad + KeepAlive，开机自启 + 崩溃自动重启
  - 标准输出/错误日志到 /var/log/
- `deploy/ios/README.md`：iOS 越狱部署完整指南
  - CA 证书生成与安装（描述文件 + 证书信任设置）
  - sing-box 二进制部署与配置
  - LaunchDaemon 加载/卸载/状态查看
  - 自研 App 调试信任（URLSessionDelegate / Info.plist）
  - 故障排查表

## Phase 8：集成测试 + 性能硬化 ✅

**完成标准（README 第 35-43 节）：**
- 端到端集成测试覆盖 TLS 终止、重写引擎、配置解析
- 性能硬化：证书 LRU 缓存、Body 大小限制、上游超时
- 生产硬化：Fail-open/Fail-closed 配置、CA 私钥权限、错误处理

**已实现：**
- `option/mitm.go`：添加 `on_error`（bypass/block）和 `upstream_timeout` 配置
- `service.go`：添加 `是否失败时绕过()` 和 `获取上游超时()` 方法
- `upstream.go`：上游 TLS 握手超时控制（默认 30 秒，可配置）
- `integration_test.go`：6 个集成测试
  - TLS 终止完整握手（客户端 → MITM → 证书验证 → 数据回显）
  - 重写引擎请求头修改
  - 重写引擎响应头 + gzip Body 替换
  - 完整配置解析（含 rewrite 规则/on_error/upstream_timeout）
  - 失败时绕过策略（bypass/block/默认）
  - 上游超时配置（默认/自定义）

## 八阶段全部完成 ✅

| 阶段 | 内容 | 状态 |
| --- | --- | --- |
| Phase 1 | 配置结构 + 服务注册 + CA 加载 | ✅ |
| Phase 2 | 动态证书 + SNI 解析 | ✅ |
| Phase 3 | TUN 拦截 + TLS 终止 + 上游连接 | ✅ |
| Phase 4 | HTTP/1.1 + HTTP/2 + WebSocket | ✅ |
| Phase 5 | Router 集成深化 + Outbound 集成 | ✅ |
| Phase 6 | Rewrite 引擎 + 压缩处理 | ✅ |
| Phase 7 | iOS 越狱层（CA/LaunchDaemon/部署文档） | ✅ |
| Phase 8 | 集成测试 + 性能硬化 + 生产硬化 | ✅ |

## 验证结果

- `go build ./cmd/sing-box` ✅ 编译通过
- `go build $(go list ./... | grep -v '/experimental/')` ✅ 全部包编译通过
- `sing-box check -c` 含 mitm 服务的配置 ✅ 校验通过
- `go test ./protocol/mitm/...` ✅ 19/19 通过（13 单元 + 6 集成）
- `sing-box generate mitm-ca` ✅ CA 证书生成验证通过（openssl 验证 CA 属性）
- GitHub Actions CI（mitm-ci.yml）✅ 全部 12 步骤 success
  - 编译主程序、编译全部包、MITM 单元测试、相关模块测试、CA 生成、配置校验、产物上传

## iOS 越狱实测阶段 ✅（部署包已就绪）

**交叉编译：**
- `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64` 编译成功（34MB Mach-O arm64）
- iOS/arm64 需要 CGO 链接，Linux 环境无法直接构建，使用 darwin/arm64 替代

**部署包内容（`deploy/ios/`）：**
- `build-ios.sh`：Linux 交叉编译脚本
- `install.sh`：iOS 设备一键安装脚本（二进制+配置+CA+LaunchDaemon）
- `config/config.json`：iOS 部署配置模板（TUN + MITM + Rewrite）
- `com.sing-box.mitm.plist`：LaunchDaemon 开机自启配置
- `README-实测指南.md`：完整实测指南（安装/CA信任/启动/验证/排障/卸载）

**部署包下载：** `sing-box-mitm-ios-deploy.tar.gz`（12MB，含预编译二进制）

**CI/CD 流水线：**
- `.github/workflows/ios-deploy.yml`：iOS 部署包自动构建
- 触发条件：sing-box 分支推送（mitm/option/deploy/ios 相关文件）或手动触发
- 构建步骤：交叉编译 darwin/arm64 → 验证 Mach-O 格式 → 组装部署包 → 验证完整性 → 打包 tar.gz → 上传 Artifact → 可选 Release
- 产物：`sing-box-mitm-ios-deploy`（24.7MB 完整部署包）+ `sing-box-darwin-arm64`（12.5MB 单独二进制）
- 首次运行：run_id 37006471392，全部 11 个步骤 success

**实测步骤摘要：**
1. SCP 传输部署包到 iOS 设备
2. 运行 `./install.sh` 一键安装
3. 安装 CA 证书到系统信任存储（描述文件 + 证书信任设置）
4. `launchctl load` 启动服务
5. Safari 访问 `https://example.com`，验证证书由 MITM CA 签发
6. 查看日志确认 MITM 拦截流程

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
        "enabled": true,
        "max_body_size": 10485760,
        "rules": [
          {
            "domain_suffix": ["example.com"],
            "path_prefix": "/api/",
            "response_header": {"X-Debug": "1"},
            "body_replace": [{"find": "old", "replace": "new"}]
          }
        ]
      }
    }
  ]
}
```
