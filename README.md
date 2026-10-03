# SFI 定制包 — 基于官方越狱版 SFI 集成 SB1 MITM 模块

## 项目说明

本分支基于官方 sing-box for Apple（SFI）越狱版，集成我们定制的 sing-box 内核（含 MITM 模块），提供官方 GUI + MITM 拦截能力。

## 官方 SFI 包结构

```
SFI-iphoneos-arm64.deb
└── var/jb/Applications/sing-box.app/
    ├── sing-box                          # 主 App 二进制（SwiftUI，36MB）
    ├── PlugIns/
    │   └── Extension.appex/
    │       └── Extension                 # PacketTunnel 扩展（Swift，118KB）
    └── Frameworks/
        └── Library.framework/
            └── Library                   # sing-box 内核动态库（95MB，Go + Swift 桥接）
```

## 定制方案

### 阶段一（当前）：流水线验证
- 下载官方 SFI deb
- 解包分析结构
- 编译我们的 sing-box 内核为 iOS 静态库（`c-archive`）
- 重新打包 deb（修改包名）
- 输出：官方重打包 deb + 定制内核静态库

### 阶段二（后续）：内核替换
- 用 Xcode 将定制静态库重新链接为 `Library.framework`
- 替换官方 framework
- ldid 签名
- 输出完整定制 deb

### 阶段三（后续）：GUI 扩展
- 在 SFI 中添加 MITM 配置页面
- 域名匹配管理
- Rewrite 规则编辑
- CA 证书安装引导

## 内核编译

官方 SFI 的内核是动态库（`Library.framework/Library`），包含 Go 运行时和 Swift 桥接代码。我们的定制内核需要：

1. 编译为 `c-archive` 静态库
2. 用 Xcode 与官方 Swift 桥接代码重新链接
3. 打包为 framework

## CI 流水线

- **文件**：`.github/workflows/sfi-custom.yml`
- **触发**：push 到 testSFI 分支，或手动触发
- **Runner**：macos-14（需要 Xcode + iOS SDK）
- **产物**：
  - `SFI-sb1mitm-deb`：定制 deb 包
  - `libsingbox-custom-a`：定制内核静态库

## 当前状态

- ✅ 官方 SFI 结构分析完成
- ✅ 流水线框架搭建完成
- ✅ 定制内核静态库编译验证
- ⏳ framework 替换（需要 Xcode 项目文件）
- ⏳ MITM 在 NetworkExtension 环境下的兼容性验证

## 相关链接

- 官方 SFI 下载：https://dl.sing-box.org/releases/latest/
- 官方客户端源码：https://github.com/SagerNet/sing-box-for-apple
- sing-box 内核源码：https://github.com/SagerNet/sing-box
