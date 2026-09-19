# Dui

Dui 是一套面向 Linux VPS 的 Xray 管理面板，提供入站、出站、路由分流、服务器运维和安全管理能力。

[![Release](https://img.shields.io/github/v/release/shini74744/Dui?style=flat-square)](https://github.com/shini74744/Dui/releases)
[![Build](https://img.shields.io/github/actions/workflow/status/shini74744/Dui/release.yml?branch=main&style=flat-square)](https://github.com/shini74744/Dui/actions)
[![Go](https://img.shields.io/github/go-mod/go-version/shini74744/Dui?style=flat-square)](go.mod)
[![Downloads](https://img.shields.io/github/downloads/shini74744/Dui/total?style=flat-square)](https://github.com/shini74744/Dui/releases)

## 主要功能

- Xray 入站、出站和路由管理
- 路由 IP / 域名支持 OR / AND
- blackmatrix7 应用规则识别与自动更新
- 自定义 Clash List 地址导入
- 规则内黑名单地址：禁止、无返回、重定向
- 多出口与负载均衡
- Cloudflare DDNS
- Fail2ban 防爆破
- NTP / 时区管理
- Swap 管理
- 证书管理与自动续签
- Telegram 管理机器人、事件通知中心与客户端独立机器人
- CPU / 内存 / 磁盘阈值告警
- 客户端到期 / 流量耗尽通知
- 每日客户端流量排名
- 来源 IP / 在线连接数实时统计
- 用户连接详情：IP 归属、运营商 / ASN、访问目标、实际出站、日志次数
- 网站 / 目标黑名单：全局或按入站 + 用户精确生效
- 来源 IP 入站黑名单与集中黑名单管理
- 星空主题、侧边栏动画与移动端适配

## v26.9.22 重点更新

- Telegram：事件通知中心、资源阈值报警、客户端独立机器人、客户端专属状态查询与手动通知历史。
- 入站监控：来源 IP / 在线连接数、连接详情、IP 归属 / ASN / ISP、访问目标、日志次数与实际出站。
- 黑名单管理：网站 / 目标支持全局或当前用户范围；来源 IP 支持整机入站拒绝；统一在 Xray 设置中集中解除。
- 首页：每日流量排名兼容单用户空 Email 入站；端口 / 用户连接数独立颜色阈值。
- UI：DUI-PRO 品牌、移动端左右留白、侧边栏收起/展开动画等细节优化。
- 安全：登录失败不再记录或发送密码；自定义客户端机器人只暴露当前客户端状态。
- 保持对现有数据库、安装路径和 `x-ui` systemd 服务名称的兼容。

## 一键安装

使用 root 用户执行：

```bash
bash <(curl -Ls https://raw.githubusercontent.com/shini74744/Dui/main/install.sh)
```
安装脚本会自动检测 CPU 架构，并从 Dui 的最新 GitHub Release 下载对应安装包。

支持的 Linux 架构：

- amd64
- arm64
- armv7
- armv6
- armv5
- 386
- s390x

## 更新

已安装的服务器执行：

```bash
x-ui
```

在菜单中选择更新即可。更新会从：

```text
https://github.com/shini74744/Dui/releases/latest
```

获取最新版本，并保留现有数据库和面板配置。

## 安装指定版本

例如安装 `v26.9.22`：

```bash
VERSION=v26.9.22 && bash <(curl -Ls "https://raw.githubusercontent.com/shini74744/Dui/${VERSION}/install.sh") ${VERSION}
```

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
> 为兼容现有安装路径与服务管理，底层程序、systemd 服务和管理命令继续使用 `x-ui` 名称；项目品牌及发布仓库统一为 Dui。

## 路由分流

Dui 的路由规则支持：

- 域名匹配
- IP / CIDR / GeoIP
- DOMAIN / DOMAIN-SUFFIX / DOMAIN-KEYWORD / DOMAIN-REGEX
- GEOSITE
- OR / AND 关系
- 单条逻辑规则在底层自动拆分
- 第三方应用规则
- 自定义公开 List URL

自定义 List 支持公开的 `http://` / `https://` 地址，并解析常见 Clash List 格式。localhost、回环地址和内网目标会被拒绝。

## 黑名单地址

每条分流规则都可以附加黑名单地址，支持：

- 域名
- IP / CIDR / GeoIP
- OR / AND
- 禁止
- 无返回
- 重定向

重定向目标未填写端口时会自动使用 `:443`，例如：

```text
ipleak.net
```

会自动规范为：

```text
ipleak.net:443
```
## GitHub Release

版本发布使用 GitHub Actions 自动完成：

```text
校验源码
→ 编译 Linux 多架构版本
→ 下载 Xray Core
→ 打包安装文件
→ 生成 SHA256
→ 创建 GitHub Release
```

Release 安装包命名保持兼容格式，例如：

```text
x-ui-linux-amd64.tar.gz
x-ui-linux-amd64.tar.gz.sha256
```

## 安全说明

- 建议使用 HTTPS 或 SSH 端口转发访问管理面板。
- 不要将数据库、证书私钥、Token、API Key 等敏感信息提交到公开仓库。
- 自定义第三方规则仅建议使用可信来源。
- 更新前建议保留 `/etc/x-ui` 数据备份。

## 项目地址

```text
https://github.com/shini74744/Dui
```

## License

本项目使用 GNU General Public License v3.0，详见 [LICENSE](LICENSE)。
