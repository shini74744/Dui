# DUI Xray vx-26.0

上游：XTLS/Xray-core v26.3.27（提交见 manifest.json），许可沿用上游 MPL-2.0。
DUI 版本与发布标签：`vx-26.0`；构建标识：`dui-vx-26.0-closewait-fix1`。
核心版本输出显示 `Xray vx-26.0`；上游兼容版本仍为 v26.3.27，并记录在 BUILD.json。

连接行为只修复 `common/singbridge/pipe.go` 的连接关闭语义：Close 中断读取并关闭写入，
CloseWrite 传递 EOF 并保留反向响应；不改变 300 秒读取超时或其他协议逻辑。
另对版本输出增加独立 DUI 版本号，保留内部协议兼容版本。附带版本输出测试和三项连接回归测试，覆盖半关闭、阻塞读退出和 CopyConn EOF 传递。

## 下载与安装

DUI v26.9.36 起的七种 Linux 安装包与 Docker 源码构建默认使用 vx-26.0。
DUI v26.9.35 起，面板版本列表和下载接口仅使用 DUI 仓库的正式核心发布，下载前匹配本机架构并验证 SHA-256。
核心 Release 单独发布并标记 make_latest=false，不影响面板安装脚本的最新版本选择。
旧版 DUI 安装包仍包含各自原有核心；升级面板会使用新版安装包内置的核心。

`targets.json` 包括上游普通发布的所有系统/架构、MIPS softfloat 和 Windows 7 专用构建。
Linux、Windows、macOS、Android、FreeBSD、OpenBSD 均提供 ZIP 和 SHA-256。
Windows 7 构建使用上游 XTLS/go-win7 的固定 1.26.1 工具链；其他平台使用官方 Go 1.26.1。
Android 使用 NDK r28b/API 24。Windows 包含 Wintun 0.14.1 及其许可证。
这些是独立核心包，不是对应系统的 DUI 面板安装包；GeoIP/GeoSite 由 DUI 单独提供。

## 可审计构建

`manifest.json` 固定上游源码压缩包 SHA-256；prepare.sh 校验后应用补丁并加入回归测试。
GitHub Actions 先运行回归测试，再按 targets.json 构建所有目标；任一失败均不发布完整核心版本。
每包包含 BUILD.json、上游 LICENSE 和本说明。Release 同时提供完整修复源码包和总校验表。
源码包包含本目录的补丁、测试和构建脚本，使用 Go 1.26.1 可重建。
Linux amd64 运行 version/run -test 冒烟验证；其他目标构建成功不代表已在对应硬件实测。

本修复解决特定 SS2022 关闭链路问题，不保证所有高内存或 CLOSE-WAIT 都由这一原因引起。
