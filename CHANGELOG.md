# Changelog

## v26.9.26 - 2026-09-22

### Xray 连接与 Socket 设置

- Xray 设置新增“内核设置”页签，并拆分为“Xray 专用”和“系统内核状态”两部分。
- Xray 专用连接生命周期支持 `handshake`、`connIdle`、`uplinkOnly`、`downlinkOnly`，仅写入 Xray policy，不修改 Linux 全局 sysctl。
- 新增 Xray Socket 策略，可按入站、出站或两者注入 KeepAlive、TCP User Timeout、TCP Fast Open、拥塞算法等参数。
- 高级 Socket 支持 TCP MSS、Window Clamp、MPTCP、出站网卡绑定、Socket Mark 和入站 IPv6 Only。
- Socket 策略只作用于 Xray 最终运行配置，不改各入站/出站数据库原始配置；关闭策略后会恢复原配置行为。
- 自动检测当前系统可用拥塞算法、网卡、TCP Fast Open 状态和 MPTCP 能力。

### 系统内核状态与安全回滚

- 系统级 sysctl 默认只读，只有用户主动进入高风险修改并二次确认后才允许写入。
- 第一次修改系统内核前固定保存最初状态到 `/var/lib/dui/kernel-tuning-original.json`，后续修改永不覆盖该备份。
- 最初备份同时记录相关 sysctl 实际值、原持久化文件是否存在及其完整内容。
- 支持“一键还原最初内核状态”，恢复第一次修改前的运行时参数和持久化文件状态。
- 内核修改和还原均执行逐项回读校验；失败时自动回滚，避免部分生效或假成功。
- 容器或非 root 环境无法修改时明确报错，不伪装成功。

### 验证

- 新增 Xray policy、Socket 注入方向、系统内核首次备份、多次修改不覆盖、原配置恢复和权限校验回归测试。
- 已验证 Xray Socket 参数可实际注入运行配置，Linux 全局 sysctl 在 Xray 专用设置前后保持不变。

## v26.9.25 - 2026-09-21

### 防爆破与保存状态修复

- SSH、TLS 异常扫描、登录失败三个 Fail2ban 规则的保存按钮改为以后端配置保存结果为准。
- 后端确认保存成功后按钮显示“已保存并启用”并变灰禁用；任意参数再次修改后自动恢复可点击状态。
- 三个 Fail2ban 规则的 `findtime`、`bantime`、`maxretry` 和白名单继续独立配置。
- 最近爆破日志按各自 `findtime` 过滤，超出检测周期的历史失败记录不再继续显示。
- 证书强制续期设置和 DDNS 检查间隔同步采用“已保存后禁用、修改后恢复”的交互。
- 新增 Fail2ban 检测周期解析与日志窗口回归测试。

## v26.9.24 - 2026-09-20

### URL 路由品牌统一

- 页面路由前缀从 `/panel/` 全面改为 `/dui/`。
- 新页面地址包括 `/dui/`、`/dui/inbounds`、`/dui/settings`、`/dui/xray` 和 `/dui/navigation`。
- API 统一改为 `/dui/api/...`；设置和 Xray 管理接口统一使用 `/dui/setting/...`、`/dui/xray/...`。
- 登录成功跳转、侧边栏、数据库下载、入站管理、Xray 管理和设置页请求全部同步切换。
- `/dui/setting/restartPanel` 同步更名为 `/dui/setting/restartDui`。
- 旧 `/panel/...` 不再注册，也不提供重定向，旧地址直接返回 404。
- 保持自定义 `webBasePath` 兼容，例如 `/abc/dui/inbounds`。
- 修复 `HttpUtil.postForm()` 使用原生 fetch 时没有继承 `webBasePath` 的问题。
- 隔离实例验证：`/dui/`、`/dui/inbounds`、`/dui/settings`、`/dui/xray`、`/dui/api/server/status` 均正常；旧 `/panel/inbounds` 返回 404。
- 自定义 `/abc/` 环境验证：`/abc/dui/inbounds` 正确注册，`/abc/panel/inbounds` 返回 404。

## v26.9.23 - 2026-09-20

### 覆盖升级兼容

- 覆盖升级已有 Dui/x-ui 时，自动识别旧安装，不再进入重新设置账号、端口和访问路径的交互流程。
- 在替换旧二进制前读取并规范化原 `webBasePath`，升级后显式恢复。
- 原路径为 `/` 时继续使用 IP/域名 + 端口直接访问，不会被新版默认 `/shlii/` 覆盖。
- 原路径为 `/abc/` 等自定义路径时继续原样保留。
- `/shlii/` 只作为全新安装的默认访问路径。
- 修复读取 CLI 设置时 ANSI 颜色码可能混入端口/路径的问题。
- 恢复旧访问路径失败会触发完整回滚，包括二进制、数据库目录、菜单脚本、systemd service 和已有 fb5。

### 连接详情兼容

- 修复旧版 Shadowsocks / Xray access.log 使用 `accepted host:port` 格式时，“连接 / 网站”为空的问题。
- 同时兼容：
  - `accepted host:port`
  - `accepted tcp:host:port`
  - `accepted udp:host:port`
- 保持来源 IP 与当前连接由系统 socket 实时统计。
- 新增日志解析回归测试，覆盖旧 Shadowsocks、新 TCP 和 UDP 三种格式。
- 已在旧面板升级实机 44569 入站验证：当前连接 33–34 条，真实 access.log 解析 847 条目标记录，Xray Configuration OK。

## v26.9.22 - 2026-09-19

### Telegram 与通知

- Telegram 设置重构为事件通知中心，新增面板名称、登录成功/失败、Fail2ban、SSH 防爆破、时区同步和 DDNS 通知。
- 新增 CPU / 内存 / 磁盘资源阈值告警，CPU/内存支持“阈值 + 持续时间”。
- 客户端支持绑定面板 Telegram 或独立 Bot Token + Chat ID。
- 客户端专属机器人可查询流量剩余、到期时间、重置时间、来源 IP 与当前连接数。
- 新增手动通知中心，可选择已绑定客户端发送主题/正文，并持久化历史通知记录。
- 登录失败不再记录或发送用户输入的密码。

### 入站与连接监控

- 入站列表新增“来源IP | 在线连接”，端口汇总与单用户分别统计。
- 来源 IP 与连接数按不同阈值独立着色，端口汇总使用更高连接阈值。
- 用户连接详情支持：
  - 来源 IP、国家/地区/城市
  - ASN / ISP / 组织
  - IP 出现次数、首次/最近出现、活跃时长
  - 当前连接数
  - 访问域名/IP、协议、端口、日志次数、最近访问
  - 实际出站标签
- 详情页支持模糊搜索、排序与刷新。

### 黑名单

- 网站/目标支持全局拉黑。
- 网站/目标支持按“入站 + 客户端 Email”精确拉黑，仅影响指定用户。
- 来源 IP 支持整机入站拒绝，并持久化恢复。
- Xray 设置新增“黑名单管理”，支持搜索、筛选、单条解除、分类清空和全部清空。
- 自动黑名单使用独立内部 blackhole 出站，不污染普通出口选择器。

### 首页与流量统计

- 新增每日客户端流量排名。
- 修复“单用户入站 + Email 为空”时今日流量无法计入排名的问题。
- 首页 Shlii 入口改为网站。
- 优化每日流量排名位置和移动端显示。

### UI / UX

- 品牌统一为 DUI-PRO。
- 登录页、首页和 Telegram 通知来源统一。
- 侧边栏收起/展开改为平滑宽度、文字和内容补位动画。
- 修复手机端首页栅格和左右留白不对称问题。
- 优化设置页按页加载，减少首屏等待。

### Xray / 路由

- 保留并增强 blackmatrix7 第三方应用规则识别。
- IP / 域名支持 OR / AND 逻辑关系。
- 规则内黑名单继续支持禁止、无返回和重定向。
- 保存 Xray 设置后自动重载；失败时尝试回滚。
- 保持多出口、负载均衡、DDNS、Fail2ban、NTP/时区、Swap、证书管理等既有功能。

### 发布

- 版本：`26.9.22`
- 支持架构：amd64、arm64、armv7、armv6、armv5、386、s390x
- Release 为每个架构同时生成 `.tar.gz` 与 `.sha256`
