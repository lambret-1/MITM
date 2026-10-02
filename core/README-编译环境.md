# 开发与编译环境说明

本目录记录 MITM 模块在云电脑上的编译自检环境配置。

## 环境一览

| 组件 | 版本 | 路径 |
| --- | --- | --- |
| Go | 1.25.5 | `../../tools/go1.25.5/` |
| sing-box | testing 分支 @ `c6c74c9` | `../../sing-box-src/` |
| GCC | 11.4.0 | 系统自带 |
| 操作系统 | Ubuntu 22.04.5 LTS (x86_64) | — |

> 系统自带 Go 为 1.23.0，不满足 sing-box testing 分支 `go 1.25.5` 的要求，因此在
> `tools/go1.25.5/` 下安装了独立工具链，不污染系统环境。

## 编译自检脚本

统一使用项目根目录下的 `tools/编译自检.sh`：

```bash
# 全量编译 sing-box 主程序并跑 go vet
./tools/编译自检.sh build

# 运行全部 Go 测试
./tools/编译自检.sh test

# 校验 JSON 配置文件
./tools/编译自检.sh check path/to/config.json
```

编译产物输出到 `tools/singbox`。

## 注意事项

- `experimental/boxdd` 与 `libbox/internal/oomprofile` 在 Linux 下存在
  `runtime/pprof.parseProcSelfMaps` 的链接问题，属于上游平台限制，不影响
  `cmd/sing-box` 主程序编译。
- `go vet` 报告的两处 `unsafe.Pointer` 警告来自上游 `daemon` / `libbox`
  代码，非本模块引入，后续修改时需保持同等谨慎。
- Go 模块代理使用 `https://goproxy.cn,direct`。
