package service

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"x-ui/database"
	"x-ui/database/model"
	"x-ui/logger"
	"x-ui/util/common"
	"x-ui/xray"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
)

type telegramEventState struct {
	sync.Mutex
	fail2banReady   bool
	ddnsReady       bool
	timeSyncReady   bool
	sshBans         map[string]bool
	panelBans       map[string]bool
	tlsBans         map[string]bool
	ddnsMarker      string
	timeSyncMarker  string
	cpuHighSince    time.Time
	cpuAlerted      bool
	memoryHighSince time.Time
	memoryAlerted   bool
	diskAlerted     bool
}

var (
	eventTelegramMu sync.RWMutex
	eventTelegram   TelegramService
	eventState      = telegramEventState{
		sshBans:   map[string]bool{},
		panelBans: map[string]bool{},
		tlsBans:   map[string]bool{},
	}
)

func SetEventTelegramService(t TelegramService) {
	eventTelegramMu.Lock()
	eventTelegram = t
	eventTelegramMu.Unlock()
}
func getEventTelegram() TelegramService {
	eventTelegramMu.RLock()
	defer eventTelegramMu.RUnlock()
	return eventTelegram
}

func telegramSettingBool(key string) bool {
	var settings SettingService
	v, err := settings.getBool(key)
	return err == nil && v
}

func telegramPanelName() string {
	var settings SettingService
	if name, err := settings.getString("tgPanelName"); err == nil {
		name = strings.TrimSpace(name)
		if name != "" {
			return name
		}
	}
	if host, err := os.Hostname(); err == nil && strings.TrimSpace(host) != "" {
		return host
	}
	return "DUI-PRO"
}

func compactText(v string, max int) string {
	v = strings.TrimSpace(strings.ReplaceAll(v, "", ""))
	if max > 0 && len([]rune(v)) > max {
		r := []rune(v)
		return string(r[:max]) + "..."
	}
	return v
}

func sendTelegramEvent(toggleKey, title string, lines ...string) {
	if toggleKey != "" && !telegramSettingBool(toggleKey) {
		return
	}
	tg := getEventTelegram()
	if tg == nil || !tg.IsRunning() {
		return
	}

	var b strings.Builder
	fmt.Fprintf(&b, "🏷 面板：%s\n", telegramPanelName())
	b.WriteString(title)
	b.WriteString("\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	if err := tg.SendMessage(strings.TrimSpace(b.String())); err != nil {
		logger.Warning("Telegram event notification failed:", err)
	}
}

func loginNotifyMasterEnabled() bool {
	return telegramSettingBool("tgBotLoginNotify")
}
func NotifyPanelLoginSuccess(username, ip, userAgent, host string) {
	if !loginNotifyMasterEnabled() {
		return
	}
	sendTelegramEvent(
		"tgNotifyLoginSuccess",
		"✅ 面板登录成功",
		"👤 账户："+compactText(username, 80),
		"🌐 IP："+compactText(ip, 120),
		"🧭 环境："+compactText(userAgent, 280),
		"🔗 Host："+compactText(host, 160),
		"🕒 时间："+time.Now().Format("2006-01-02 15:04:05"),
	)
}

func NotifyPanelLoginFailure(username, ip, userAgent, host string) {
	if !loginNotifyMasterEnabled() {
		return
	}
	sendTelegramEvent(
		"tgNotifyLoginFail",
		"⚠️ 面板登录失败",
		"👤 尝试账户："+compactText(username, 80),
		"🌐 IP："+compactText(ip, 120),
		"🧭 环境："+compactText(userAgent, 280),
		"🔗 Host："+compactText(host, 160),
		"🕒 时间："+time.Now().Format("2006-01-02 15:04:05"),
	)
}
func stringSet(items []string) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			out[item] = true
		}
	}
	return out
}

func setDiff(current, previous map[string]bool) (added, removed []string) {
	for item := range current {
		if !previous[item] {
			added = append(added, item)
		}
	}
	for item := range previous {
		if !current[item] {
			removed = append(removed, item)
		}
	}
	return
}

func notifyFail2banDiff(toggleKey, label string, current, previous map[string]bool) {
	added, removed := setDiff(current, previous)
	for _, ip := range added {
		sendTelegramEvent(
			toggleKey,
			"🚫 防爆破已封禁",
			"🛡 类型："+label,
			"🌐 IP："+ip,
			"🕒 时间："+time.Now().Format("2006-01-02 15:04:05"),
		)
	}
	for _, ip := range removed {
		sendTelegramEvent(
			toggleKey,
			"♻️ 防爆破解封",
			"🛡 类型："+label,
			"🌐 IP："+ip,
			"🕒 时间："+time.Now().Format("2006-01-02 15:04:05"),
		)
	}
}

func pollFail2banTelegramEvents() {
	if !telegramSettingBool("tgNotifySSHBruteForce") &&
		!telegramSettingBool("tgNotifyPanelBruteForce") {
		return
	}
	var svc Fail2banService
	status, err := svc.Status()
	if err != nil || status == nil || !status.Running {
		return
	}

	ssh := stringSet(status.SSHD.BannedIPs)
	panel := stringSet(status.XUILogin.BannedIPs)
	tls := stringSet(status.XUITLS.BannedIPs)

	eventState.Lock()
	if !eventState.fail2banReady {
		eventState.sshBans = ssh
		eventState.panelBans = panel
		eventState.tlsBans = tls
		eventState.fail2banReady = true
		eventState.Unlock()
		return
	}
	oldSSH := eventState.sshBans
	oldPanel := eventState.panelBans
	oldTLS := eventState.tlsBans
	eventState.sshBans = ssh
	eventState.panelBans = panel
	eventState.tlsBans = tls
	eventState.Unlock()

	notifyFail2banDiff("tgNotifySSHBruteForce", "SSH / sshd", ssh, oldSSH)
	notifyFail2banDiff("tgNotifyPanelBruteForce", "面板登录 / 3xui-login", panel, oldPanel)
	notifyFail2banDiff("tgNotifyPanelBruteForce", "面板 TLS / 3xui-tls", tls, oldTLS)
}
func ddnsRunMarker(st *DDNSStatus) string {
	if st == nil || st.LastRun == nil {
		return ""
	}
	return strings.TrimSpace(st.LastRun.CheckedAt)
}

func sendDDNSStatusEvent(source string, st *DDNSStatus) {
	if st == nil || st.LastRun == nil {
		return
	}
	run := st.LastRun
	title := "✅ DDNS 同步完成"
	if !strings.EqualFold(run.Status, "ok") &&
		!strings.EqualFold(run.Status, "updated") &&
		!strings.EqualFold(run.Status, "unchanged") {
		title = "⚠️ DDNS 同步异常"
	}
	sendTelegramEvent(
		"tgNotifyDDNS",
		title,
		"🔄 来源："+source,
		"🌐 出口 IPv4："+compactText(run.PublicIPv4, 100),
		"📌 状态："+compactText(run.Status, 100),
		"📝 说明："+compactText(run.Message, 300),
		"🕒 检查时间："+compactText(run.CheckedAt, 100),
	)
}

func NotifyDDNSCurrent(source string) {
	if !telegramSettingBool("tgNotifyDDNS") {
		return
	}
	var svc DDNSService
	st, err := svc.Status()
	if err != nil || st == nil || st.LastRun == nil {
		return
	}
	marker := ddnsRunMarker(st)
	eventState.Lock()
	eventState.ddnsMarker = marker
	eventState.ddnsReady = true
	eventState.Unlock()
	sendDDNSStatusEvent(source, st)
}

func NotifyDDNSAction(source string, err error) {
	if !telegramSettingBool("tgNotifyDDNS") {
		return
	}
	if err != nil {
		sendTelegramEvent(
			"tgNotifyDDNS",
			"⚠️ DDNS 同步失败",
			"🔄 来源："+source,
			"📝 错误："+compactText(err.Error(), 320),
			"🕒 时间："+time.Now().Format("2006-01-02 15:04:05"),
		)
		return
	}
	NotifyDDNSCurrent(source)
}

func pollDDNSTelegramEvents() {
	if !telegramSettingBool("tgNotifyDDNS") {
		return
	}
	var svc DDNSService
	st, err := svc.Status()
	if err != nil || st == nil || st.LastRun == nil {
		return
	}
	marker := ddnsRunMarker(st)
	if marker == "" {
		return
	}

	eventState.Lock()
	if !eventState.ddnsReady {
		eventState.ddnsMarker = marker
		eventState.ddnsReady = true
		eventState.Unlock()
		return
	}
	changed := marker != eventState.ddnsMarker
	if changed {
		eventState.ddnsMarker = marker
	}
	eventState.Unlock()
	if changed {
		sendDDNSStatusEvent("自动任务", st)
	}
}
func currentTimeSyncRunMarker() (marker, result string) {
	if !commandExists("systemctl") {
		return "", ""
	}
	marker, _ = runSystemCommand(
		5*time.Second,
		"systemctl", "show", "time-sync-manager.service",
		"-p", "ExecMainExitTimestampMonotonic", "--value",
	)
	result, _ = runSystemCommand(
		5*time.Second,
		"systemctl", "show", "time-sync-manager.service",
		"-p", "Result", "--value",
	)
	return strings.TrimSpace(marker), strings.TrimSpace(result)
}

func sendTimeSyncStatusEvent(source string, success bool, detail string) {
	var svc TimeSyncService
	st, _ := svc.Status()
	lines := []string{"🔄 来源：" + source}
	if st != nil {
		lines = append(lines,
			"🌍 当前时区："+compactText(st.SystemTimezone, 120),
			fmt.Sprintf("⏱ NTP 同步：%v", st.NTPSynchronized),
			"🕒 本机时间："+compactText(st.LocalTime, 120),
		)
	}
	if strings.TrimSpace(detail) != "" {
		lines = append(lines, "📝 结果："+compactText(detail, 320))
	}
	title := "✅ 时区/时间同步完成"
	if !success {
		title = "⚠️ 时区/时间同步失败"
	}
	sendTelegramEvent("tgNotifyTimeSync", title, lines...)
}

func NotifyTimeSyncAction(source string, err error) {
	if !telegramSettingBool("tgNotifyTimeSync") {
		return
	}
	marker, _ := currentTimeSyncRunMarker()
	eventState.Lock()
	if marker != "" {
		eventState.timeSyncMarker = marker
		eventState.timeSyncReady = true
	}
	eventState.Unlock()
	if err != nil {
		sendTimeSyncStatusEvent(source, false, err.Error())
	} else {
		sendTimeSyncStatusEvent(source, true, "")
	}
}

func pollTimeSyncTelegramEvents() {
	if !telegramSettingBool("tgNotifyTimeSync") {
		return
	}
	marker, result := currentTimeSyncRunMarker()
	if marker == "" || marker == "0" {
		return
	}
	eventState.Lock()
	if !eventState.timeSyncReady {
		eventState.timeSyncMarker = marker
		eventState.timeSyncReady = true
		eventState.Unlock()
		return
	}
	changed := marker != eventState.timeSyncMarker
	if changed {
		eventState.timeSyncMarker = marker
	}
	eventState.Unlock()
	if changed {
		sendTimeSyncStatusEvent("自动定时任务", result == "success", result)
	}
}

func pollClientLimitTelegramEvents() {
	db := database.GetDB()
	if db == nil {
		return
	}

	var inbounds []model.Inbound
	if err := db.Find(&inbounds).Error; err != nil {
		logger.Warning("load inbounds for client notify failed:", err)
		return
	}

	var traffics []xray.ClientTraffic
	if err := db.Find(&traffics).Error; err != nil {
		logger.Warning("load client traffic for notify failed:", err)
		return
	}
	trafficByEmail := make(map[string]xray.ClientTraffic, len(traffics))
	for _, tr := range traffics {
		trafficByEmail[tr.Email] = tr
	}

	now := time.Now().UnixMilli()
	for _, inbound := range inbounds {
		if strings.TrimSpace(inbound.Settings) == "" {
			continue
		}
		var settings struct {
			Clients []struct {
				Email      string `json:"email"`
				TgNotify   bool   `json:"tgNotify"`
				TotalGB    int64  `json:"totalGB"`
				ExpiryTime int64  `json:"expiryTime"`
			} `json:"clients"`
		}
		if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
			continue
		}
		for _, client := range settings.Clients {
			if !client.TgNotify || strings.TrimSpace(client.Email) == "" {
				continue
			}
			if client.TotalGB <= 0 && client.ExpiryTime == 0 {
				continue
			}

			state := model.ClientNotifyState{InboundId: inbound.Id, Email: client.Email}
			if err := db.Where("inbound_id = ? AND email = ?", inbound.Id, client.Email).
				FirstOrCreate(&state).Error; err != nil {
				continue
			}

			traffic := trafficByEmail[client.Email]
			expired := client.ExpiryTime > 0 && now >= client.ExpiryTime
			exhausted := client.TotalGB > 0 && (traffic.Up+traffic.Down) >= client.TotalGB

			changed := false
			if !expired && state.ExpiryNotified {
				state.ExpiryNotified = false
				changed = true
			}
			if !exhausted && state.TrafficNotified {
				state.TrafficNotified = false
				changed = true
			}

			if expired && !state.ExpiryNotified {
				sendTelegramEvent(
					"",
					"⏰ 客户端已到期",
					"📌 入站："+compactText(inbound.Remark, 120),
					fmt.Sprintf("🔌 端口：%d", inbound.Port),
					"👤 用户："+compactText(client.Email, 120),
					"🕒 到期时间："+time.UnixMilli(client.ExpiryTime).Format("2006-01-02 15:04:05"),
				)
				state.ExpiryNotified = true
				changed = true
			}

			if exhausted && !state.TrafficNotified {
				used := traffic.Up + traffic.Down
				sendTelegramEvent(
					"",
					"📦 客户端流量已用完",
					"📌 入站："+compactText(inbound.Remark, 120),
					fmt.Sprintf("🔌 端口：%d", inbound.Port),
					"👤 用户："+compactText(client.Email, 120),
					"📊 已用："+common.FormatTraffic(used),
					"🎯 总量："+common.FormatTraffic(client.TotalGB),
				)
				state.TrafficNotified = true
				changed = true
			}

			if changed {
				state.UpdatedAt = time.Now().Unix()
				_ = db.Save(&state).Error
			}
		}
	}
}

func telegramSettingInt(key string, fallback int) int {
	var settings SettingService
	v, err := settings.getInt(key)
	if err != nil {
		return fallback
	}
	return v
}

func pollResourceTelegramEvents() {
	cpuEnabled := telegramSettingBool("tgNotifyCPU")
	memEnabled := telegramSettingBool("tgNotifyMemory")
	diskEnabled := telegramSettingBool("tgNotifyDisk")
	if !cpuEnabled && !memEnabled && !diskEnabled {
		eventState.Lock()
		eventState.cpuHighSince = time.Time{}
		eventState.cpuAlerted = false
		eventState.memoryHighSince = time.Time{}
		eventState.memoryAlerted = false
		eventState.diskAlerted = false
		eventState.Unlock()
		return
	}

	now := time.Now()

	if cpuEnabled {
		threshold := telegramSettingInt("tgCPUThreshold", 90)
		duration := time.Duration(telegramSettingInt("tgCPUDuration", 300)) * time.Second
		values, err := cpu.Percent(750*time.Millisecond, false)
		if err == nil && len(values) > 0 {
			value := values[0]
			eventState.Lock()
			if value >= float64(threshold) {
				if eventState.cpuHighSince.IsZero() {
					eventState.cpuHighSince = now
				}
				shouldAlert := !eventState.cpuAlerted && now.Sub(eventState.cpuHighSince) >= duration
				if shouldAlert {
					eventState.cpuAlerted = true
				}
				eventState.Unlock()
				if shouldAlert {
					sendTelegramEvent(
						"tgNotifyCPU",
						"🔥 CPU 使用率报警",
						fmt.Sprintf("📈 当前：%.1f%%", value),
						fmt.Sprintf("⚠️ 阈值：%d%%", threshold),
						fmt.Sprintf("⏱ 持续：%s", duration),
					)
				}
			} else {
				wasAlerted := eventState.cpuAlerted
				eventState.cpuHighSince = time.Time{}
				eventState.cpuAlerted = false
				eventState.Unlock()
				if wasAlerted {
					sendTelegramEvent(
						"tgNotifyCPU",
						"✅ CPU 使用率已恢复",
						fmt.Sprintf("📉 当前：%.1f%%", value),
						fmt.Sprintf("⚠️ 阈值：%d%%", threshold),
					)
				}
			}
		}
	} else {
		eventState.Lock()
		eventState.cpuHighSince = time.Time{}
		eventState.cpuAlerted = false
		eventState.Unlock()
	}

	if memEnabled {
		threshold := telegramSettingInt("tgMemoryThreshold", 90)
		duration := time.Duration(telegramSettingInt("tgMemoryDuration", 300)) * time.Second
		if info, err := mem.VirtualMemory(); err == nil {
			value := info.UsedPercent
			eventState.Lock()
			if value >= float64(threshold) {
				if eventState.memoryHighSince.IsZero() {
					eventState.memoryHighSince = now
				}
				shouldAlert := !eventState.memoryAlerted && now.Sub(eventState.memoryHighSince) >= duration
				if shouldAlert {
					eventState.memoryAlerted = true
				}
				eventState.Unlock()
				if shouldAlert {
					sendTelegramEvent(
						"tgNotifyMemory",
						"🧠 内存使用率报警",
						fmt.Sprintf("📈 当前：%.1f%%", value),
						fmt.Sprintf("⚠️ 阈值：%d%%", threshold),
						fmt.Sprintf("⏱ 持续：%s", duration),
					)
				}
			} else {
				wasAlerted := eventState.memoryAlerted
				eventState.memoryHighSince = time.Time{}
				eventState.memoryAlerted = false
				eventState.Unlock()
				if wasAlerted {
					sendTelegramEvent(
						"tgNotifyMemory",
						"✅ 内存使用率已恢复",
						fmt.Sprintf("📉 当前：%.1f%%", value),
						fmt.Sprintf("⚠️ 阈值：%d%%", threshold),
					)
				}
			}
		}
	} else {
		eventState.Lock()
		eventState.memoryHighSince = time.Time{}
		eventState.memoryAlerted = false
		eventState.Unlock()
	}

	if diskEnabled {
		threshold := telegramSettingInt("tgDiskThreshold", 90)
		if info, err := disk.Usage("/"); err == nil {
			value := info.UsedPercent
			eventState.Lock()
			if value >= float64(threshold) {
				shouldAlert := !eventState.diskAlerted
				eventState.diskAlerted = true
				eventState.Unlock()
				if shouldAlert {
					sendTelegramEvent(
						"tgNotifyDisk",
						"💾 磁盘使用率报警",
						fmt.Sprintf("📈 根分区当前：%.1f%%", value),
						fmt.Sprintf("⚠️ 阈值：%d%%", threshold),
					)
				}
			} else {
				wasAlerted := eventState.diskAlerted
				eventState.diskAlerted = false
				eventState.Unlock()
				if wasAlerted {
					sendTelegramEvent(
						"tgNotifyDisk",
						"✅ 磁盘使用率已恢复",
						fmt.Sprintf("📉 根分区当前：%.1f%%", value),
						fmt.Sprintf("⚠️ 阈值：%d%%", threshold),
					)
				}
			}
		}
	} else {
		eventState.Lock()
		eventState.diskAlerted = false
		eventState.Unlock()
	}
}

func PollTelegramEventNotifications() {
	tg := getEventTelegram()
	if tg == nil || !tg.IsRunning() {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("Telegram event monitor panic: %v", r)
		}
	}()
	pollFail2banTelegramEvents()
	pollDDNSTelegramEvents()
	pollTimeSyncTelegramEvents()
	pollResourceTelegramEvents()
}
