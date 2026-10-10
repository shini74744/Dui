# DUI-PRO（Dui）

Dui 是面向 Linux VPS 的 Xray 管理面板，提供入站、出站、路由分流、负载均衡、连接统计和服务器管理功能。面板、DUI 核心及辅助协议组件在同一仓库发布。

[![Release](https://img.shields.io/github/v/release/shini74744/Dui?style=flat-square)](https://github.com/shini74744/Dui/releases)
[![Build](https://img.shields.io/github/actions/workflow/status/shini74744/Dui/release.yml?branch=main&style=flat-square)](https://github.com/shini74744/Dui/actions)
[![Go](https://img.shields.io/github/go-mod/go-version/shini74744/Dui?style=flat-square)](go.mod)
[![Downloads](https://img.shields.io/github/downloads/shini74744/Dui/total?style=flat-square)](https://github.com/shini74744/Dui/releases)

## 当前发布版本

文档核对日期：2026-10-07。后续版本请以 [Releases](https://github.com/shini74744/Dui/releases) 为准。

| 组件 | 版本 | 说明 |
| --- | --- | --- |
| 面板 | [v26.9.51](https://github.com/shini74744/Dui/releases/tag/v26.9.51) | 修复首页更新按钮的表单请求解析；包含已安装核心识别、旧版本升级兼容及后台下载进度 |
| DUI 核心 | [vx-26.7](https://github.com/shini74744/Dui/releases/tag/vx-26.7) | 基于 Xray v26.9.9；支持显式配置的公网明文 VLESS 出站 |
| 辅助协议组件 | 随面板安装包发布 | TUIC、MTProto 与 AmneziaWG 的能力和平台支持见 [组件说明](core/helpers/README.md) |

v26.9.52–v26.9.53（待发布）新增 [出站下载测速](docs/outbound-speedtest.md)、策略探测折叠菜单及新配置默认 30 秒间隔。已保存的检测间隔不会自动覆盖。测速页同时提供紧凑布局、实际采样进度和不完整结果提示。

待发布核心 vx-26.8 将四种策略的故障出口排除与备用出口解耦；备用出口留空且全部候选出口不可用时拒绝新连接，探测恢复后自动重新使用。详见[策略探测与故障切换](docs/strategy-probes.md)。

面板的 `v26.9.*` 与核心的 `vx-26.*` 独立编号。首页分别显示两个组件的已安装版本与可用更新；核心的上游版本、构建来源记录在 `BUILD.json` 中。

## 使用文档

| 内容 | 文档 |
| --- | --- |
| 首页更新提示、后台下载、完整性校验、失败恢复和旧版升级 | [后台更新](docs/verified-updates.md) |
| 出站改名后同步路由、负载均衡、探测和链式出站引用 | [出站同步保存](docs/outbound-reference-sync.md) |
| 鼠标/手机拖动出站排序、默认出口 | [出站排序](docs/outbound-order.md) |
| 四种策略独立探测、检测间隔、最近 7 天记录 | [策略探测](docs/strategy-probes.md) |
| 自维护核心特性、版本兼容和构建来源 | [DUI Xray 核心](core/xray/README.md) |
| 辅助协议的路由、用户控制和平台能力 | [协议配套组件](core/helpers/README.md) |
| 历史更新 | [更新日志](CHANGELOG.md) |

## 主要功能

- **协议管理**：VLESS、VMess、Trojan、Shadowsocks/SS2022、WireGuard，以及 Hysteria 2、Mixed、TUN；TUIC、MTProto、AmneziaWG 由配套组件提供。
- **出站和路由**：出站拖动排序、编辑时同步引用、域名/IP 分流、OR/AND 条件、第三方应用规则、自定义公开 Clash List 标签及自动更新。
- **负载均衡**：最低延迟、最低负载、随机、轮询分别配置探测参数；界面跟随面板语言；真实探测记录支持筛选和分页。
- **更新管理**：首页卡片内检查面板/核心更新，绿色“更新版本”提示，后台下载进度，下载及校验完成后才替换。
- **用户和连接**：流量、到期、协议支持范围内的独立上下行 Mbps 限速；来源 IP、在线连接数、访问目标、实际出站和每日流量排名。
- **黑名单**：网站/目标可全局或按入站及用户生效；支持禁止、无返回、重定向和来源 IP 集中管理。
- **服务器管理**：Cloudflare DDNS、证书续期、Fail2ban、NTP/时区、Swap、资源告警和 Telegram 通知。
- **界面**：明暗及星空主题、移动端适配、居中的手机二维码弹窗。

各协议支持的用户控制并不完全相同。例如 TUIC、MTProto 采用直连出口，不套用 Xray 分流规则；具体边界见 [协议配套组件](core/helpers/README.md)。

## 安装

使用 root 用户执行：

```bash
bash <(curl -Ls https://raw.githubusercontent.com/shini74744/Dui/main/install.sh)
```

脚本自动识别架构，并选择正式发布的面板安装包。支持 `amd64`、`arm64`、`armv7`、`armv6`、`armv5`、`386`、`s390x`。辅助组件的预编译平台范围见 [组件说明](core/helpers/README.md)。

安装指定版本，例如：

```bash
VERSION=v26.9.51
bash <(curl -Ls "https://raw.githubusercontent.com/shini74744/Dui/${VERSION}/install.sh") "${VERSION}"
```

全新安装按交互提示设置账号、密码、端口和访问路径；默认访问路径为 `/shlii/`。覆盖升级会保留原账号、密码、端口、证书和 `webBasePath`。原路径为 `/` 时仍使用原来的“域名/IP + 端口”登录，不会自动添加访问路径。

底层程序、systemd 服务和管理命令继续使用 `x-ui` 名称，以兼容现有安装。

## 更新

### 在首页更新

打开首页的 **DUI-PRO 面板卡片**：

1. 没有可用更新时显示“检查更新”；有更新时显示绿色“更新版本”。
2. 点击后直接在卡片内查看面板/核心版本，并选择对应组件的“后台下载并更新”。
3. 卡片显示下载字节、总大小、百分比和处理阶段。关闭或刷新页面不取消后台任务。
4. 下载期间现有服务继续运行；完整性、版本及必要配置校验通过后，安装阶段会短暂重启服务。

**首页“面板更新”只替换面板程序，“内核更新”只替换核心程序。** 如果新功能要求更高核心版本，需要分别更新。完整安装包还包含菜单脚本、辅助组件等文件；更新这些配套文件请使用安装脚本或相应部署流程。

后台覆盖更新适用于 Linux、root 和标准 `x-ui.service` 安装。Docker 等部署方式通过自身部署流程更新。详细流程和故障说明见 [后台更新](docs/verified-updates.md)。

### 从旧版或命令行更新

```bash
x-ui
```

在菜单中选择更新，也可执行：

```bash
x-ui update
```

如果旧版本的首页按钮提示 `invalid character 'k' looking for beginning of value`，可通过上述命令行入口先升级面板；v26.9.51 已修复该问题。若旧菜单本身无法完成升级，使用本页安装脚本覆盖升级。

升级前保留 `/etc/x-ui` 的可恢复备份，以及外部证书等自定义文件。下载或校验失败时，不应手工用未下载完成的文件覆盖正在运行的程序。

## 出站与路由使用要点

- 修改出站地址、端口、密码等参数且标签不变时，原路由继续通过同一标签引用该出站。
- 修改标签时，可用“同步保存”一并更新相关引用；弹窗确认后仍需点击页面“保存”才生效。
- 先点普通“确定”后，可以在同一页面草稿中重新编辑并补做同步。待同步记录不会跨页面刷新保存。
- 拖动排序改变的是出站列表顺序。**未命中任何路由时使用首个出站**；需要默认直连时，应把 `freedom/direct` 出站放在第一位。
- 路由规则从上到下匹配。宽泛规则放在前面可能覆盖后面的具体规则；内网或 BitTorrent 拦截规则也遵循该顺序。
- 最低负载根据探测延迟、波动和失败情况等指标选择出口，不读取远端 CPU、内存、在线人数或剩余带宽。

## 第三方规则与黑名单

路由支持域名、IP/CIDR/GeoIP、GEOSITE，以及 DOMAIN、DOMAIN-SUFFIX、DOMAIN-KEYWORD、DOMAIN-REGEX 等常见 Clash List 规则。域名和 IP 可按 OR/AND 组合；一条逻辑规则可能在底层拆分为多条核心规则。

自定义标签可保存多个公开 `http://` / `https://` List 地址，批量选择、合并并去重。为避免读取服务器内部资源，localhost、回环及内网目标会被拒绝。路由来源信息保存在 `/etc/x-ui/route_rule_sources.json`，不写入 Xray 配置。

黑名单可用于全局或指定入站/用户，支持禁止、无返回、重定向。重定向目标未填写端口时默认补充 `:443`。

## 日志与连接生命周期

首次安装默认日志等级为 `warning`，访问日志保持启用；已有安装升级保留原日志设置。来源 IP、连接详情等功能可能依赖访问日志，调整前应确认用途。

Xray 设置提供 `handshake`、`connIdle`、`uplinkOnly`、`downlinkOnly` 和 Socket 参数。DUI 核心还提供可选的 Linux CLOSE-WAIT 清理超时（0 关闭，1–600 秒启用），需要 vx-26.4 或更新版本。它只处理本核心拥有且持续处于 CLOSE-WAIT 的 TCP 连接；可能截断合法的半关闭传输，不能作为内存泄漏或所有下载正常的判据。实现细节见 [核心说明](core/xray/README.md)。

## 常用命令

```bash
x-ui start
x-ui stop
x-ui restart
x-ui status
x-ui enable
x-ui disable
x-ui log
x-ui update
```

## 发布与源码

面板发布包沿用兼容命名：

```text
x-ui-linux-amd64.tar.gz
x-ui-linux-amd64.tar.gz.sha256
```

GitHub Actions 校验源码、构建各平台、组装已校验的 DUI 核心和辅助组件，并发布安装包及 SHA-256。核心使用独立发布流程，提供各平台 ZIP、校验文件、`SHA256SUMS` 和完整修改源码 `DUI-Xray-source.tar.gz`。

- [面板构建流程](.github/workflows/release.yml)
- [核心构建流程](.github/workflows/xray-core.yml)
- [源码来源与许可证记录](internal/PROVENANCE.md)

## 安全与许可证

建议通过 HTTPS 或 SSH 转发访问管理面板。数据库、证书私钥、Token、API Key 和节点凭据不应提交到公开仓库；第三方规则使用可信来源。

本项目使用 GNU General Public License v3.0，详见 [LICENSE](LICENSE)。
