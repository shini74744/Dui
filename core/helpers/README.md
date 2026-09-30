# DUI 协议配套组件

面板和核心的功能需要配套使用。核心仍基于 XTLS/Xray-core v26.9.9；辅助协议的生命周期、配置校验、流量和用户管理按 DUI 的数据库与服务接口适配。参考的 3x-ui 源码固定在 `8c023d13dc9d01a72bf8be6fa409c6af392b8088`，来源记录在 `internal/PROVENANCE.md`。

| 协议 | 实现与出口 | 支持的用户控制 | 客户端配置 |
| --- | --- | --- | --- |
| Hysteria 2 | DUI Xray，经过 Xray 路由；UDP + TLS | 开关、到期、配额、用户 Mbps 限速；撤销已有 QUIC 会话 | `hysteria2://` |
| Mixed | DUI Xray，同端口 HTTP/SOCKS5；经过 Xray 路由 | 入站流量和到期、密码认证；不提供单用户配额 | HTTP/SOCKS5 参数 |
| TUN | DUI Xray，本机虚拟网卡；经过 Xray 路由 | 入站设置；没有用户节点链接 | 本机网卡、地址、可选系统路由 |
| TUIC v5 | 固定上游 tuic-server，直连出口 | 用户开关、到期；统计入站总流量；不支持单用户字节配额、Mbps 限速或来源 IP 数限制 | `tuic://` |
| MTProto | 固定上游 mtg-multi，直连出口 | 多用户 secret、开关、到期和流量配额；不支持 Mbps 限速或来源 IP 数限制 | Telegram `tg://proxy` |
| AmneziaWG | 面板内嵌用户态实现，经过本机 Xray 转发和路由 | 用户开关、到期和流量配额；不支持 Mbps 限速、来源 IP 数限制或公网 IPv6 绑定 | 专用 `.conf`，不是 Xray JSON |

Hysteria 2 需要可用的 TLS 证书和 UDP 端口。TUN 需要 Linux `/dev/net/tun` 和相应网络权限；默认不写系统路由。TUIC 和 MTProto 的直连出口不会套用面板里的 Xray 分流规则。辅助协议不支持的控制项会隐藏，并由后端拒绝，避免保存后没有实际效果。

`manifest.json` 固定组件版本、官方 GitHub 资源和 SHA-256；`assemble.py` 校验归档后仅读取指定的普通文件，不提取归档内路径或符号链接。发布包保存组件来源、许可证及二进制校验值。面板仅使用自己的 `bin` 目录中的组件，不接管机器上的其他同名服务。

| 面板平台 | TUIC 预编译组件 | MTProto 预编译组件 |
| --- | --- | --- |
| amd64、arm64、armv7、386 | 有 | 有 |
| armv5、armv6、s390x | 上游没有对应发布资源；界面提示缺少组件 | 有 |

所有平台的包都会记录实际可用组件；不把其他架构的程序装作兼容组件。AmneziaWG 随面板构建。各平台编译通过不等于每个平台都完成了真实网络测试。

回归脚本在隔离目录和回环端口运行：`scripts/test-native-integration.py` 测试新增协议的数据传输、用户撤销、配额、恢复、重启和错误配置回滚；AmneziaWG 在独立网络命名空间测试。`scripts/test-panel-compatibility.py` 测试 VLESS Encryption 两种认证、TCP/XHTTP、REALITY 兼容模式切换和用户限速热更新。两者使用独立临时数据库，不修改已安装面板的数据。
