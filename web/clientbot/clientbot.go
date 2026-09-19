package clientbot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"x-ui/database"
	"x-ui/database/model"
	"x-ui/logger"
	"x-ui/util/common"
	"x-ui/web/job"
	"x-ui/xray"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
	"gorm.io/gorm"
)

const (
	ModePanel  = "panel"
	ModeCustom = "custom"
)

type ConfigView struct {
	InboundID        int    `json:"inboundId"`
	Email            string `json:"email"`
	Enabled          bool   `json:"enabled"`
	Mode             string `json:"mode"`
	BotToken         string `json:"botToken"`
	ChatID           int64  `json:"chatId"`
	Running          bool   `json:"running"`
	BotUsername      string `json:"botUsername"`
	LastError        string `json:"lastError"`
	PanelAvailable   bool   `json:"panelAvailable"`
	PanelBotUsername string `json:"panelBotUsername"`
}

type clientSnapshot struct {
	Email      string `json:"email"`
	TotalGB    int64  `json:"totalGB"`
	ExpiryTime int64  `json:"expiryTime"`
	Reset      int    `json:"reset"`
	Enable     bool   `json:"enable"`
}

type runner struct {
	key      string
	cfg      model.ClientTelegramBot
	bot      *telego.Bot
	username string
	cancel   context.CancelFunc
	lastErr  string
}

var manager = struct {
	sync.Mutex
	runners       map[string]*runner
	started       bool
	panelBot      *telego.Bot
	panelUsername string
}{
	runners: map[string]*runner{},
}

func key(inboundID int, email string) string {
	return fmt.Sprintf("%d|%s", inboundID, strings.ToLower(strings.TrimSpace(email)))
}

func normalizeMode(mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), ModeCustom) {
		return ModeCustom
	}
	return ModePanel
}

func compact(v string, max int) string {
	r := []rune(strings.TrimSpace(v))
	if len(r) <= max {
		return string(r)
	}
	return string(r[:max]) + "..."
}

func SetPanelBot(bot *telego.Bot, username string) {
	manager.Lock()
	manager.panelBot = bot
	manager.panelUsername = strings.TrimPrefix(strings.TrimSpace(username), "@")
	manager.Unlock()
}

func panelBotInfo() (*telego.Bot, string) {
	manager.Lock()
	defer manager.Unlock()
	return manager.panelBot, manager.panelUsername
}

func Start() {
	manager.Lock()
	if manager.started {
		manager.Unlock()
		return
	}
	manager.started = true
	manager.Unlock()

	go func() {
		Reconcile()
		pollNotifications()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			Reconcile()
			pollNotifications()
		}
	}()
}

func Stop() {
	manager.Lock()
	defer manager.Unlock()
	for k, r := range manager.runners {
		if r.cancel != nil {
			r.cancel()
		}
		delete(manager.runners, k)
	}
	manager.started = false
	manager.panelBot = nil
	manager.panelUsername = ""
}

func Reconcile() {
	db := database.GetDB()
	if db == nil {
		return
	}

	var configs []model.ClientTelegramBot
	if err := db.Where("enabled = ? AND mode = ?", true, ModeCustom).Find(&configs).Error; err != nil {
		logger.Warning("client bot reconcile failed:", err)
		return
	}

	wanted := map[string]model.ClientTelegramBot{}
	for _, cfg := range configs {
		if strings.TrimSpace(cfg.Email) == "" || strings.TrimSpace(cfg.BotToken) == "" || cfg.ChatId == 0 {
			continue
		}
		cfg.Mode = ModeCustom
		wanted[key(cfg.InboundId, cfg.Email)] = cfg
	}

	manager.Lock()
	for k, r := range manager.runners {
		cfg, ok := wanted[k]
		if !ok || cfg.BotToken != r.cfg.BotToken || cfg.ChatId != r.cfg.ChatId {
			if r.cancel != nil {
				r.cancel()
			}
			delete(manager.runners, k)
		}
	}
	manager.Unlock()

	for k, cfg := range wanted {
		manager.Lock()
		_, exists := manager.runners[k]
		manager.Unlock()
		if !exists {
			startCustomRunner(cfg)
		}
	}
}

func startCustomRunner(cfg model.ClientTelegramBot) {
	bot, err := telego.NewBot(strings.TrimSpace(cfg.BotToken))
	if err != nil {
		setRunnerError(key(cfg.InboundId, cfg.Email), cfg, err.Error())
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	me, err := bot.GetMe(ctx)
	if err != nil {
		cancel()
		setRunnerError(key(cfg.InboundId, cfg.Email), cfg, err.Error())
		return
	}

	_ = bot.SetMyCommands(ctx, &telego.SetMyCommandsParams{
		Commands: []telego.BotCommand{
			{Command: "start", Description: "查看当前客户端状态"},
			{Command: "status", Description: "查看流量、到期和连接信息"},
			{Command: "info", Description: "查看当前客户端状态"},
			{Command: "help", Description: "查看可用命令"},
		},
	})

	k := key(cfg.InboundId, cfg.Email)
	r := &runner{
		key:      k,
		cfg:      cfg,
		bot:      bot,
		username: me.Username,
		cancel:   cancel,
	}
	manager.Lock()
	manager.runners[k] = r
	manager.Unlock()

	go runCustomBot(ctx, r)
}

func setRunnerError(k string, cfg model.ClientTelegramBot, msg string) {
	manager.Lock()
	defer manager.Unlock()
	manager.runners[k] = &runner{
		key:     k,
		cfg:     cfg,
		lastErr: compact(msg, 180),
	}
}

func setRunnerRuntimeError(k string, err error) {
	manager.Lock()
	defer manager.Unlock()
	if r := manager.runners[k]; r != nil {
		r.lastErr = compact(err.Error(), 180)
	}
}

func runCustomBot(ctx context.Context, r *runner) {
	updates, err := r.bot.UpdatesViaLongPolling(ctx, &telego.GetUpdatesParams{Timeout: 10})
	if err != nil {
		setRunnerRuntimeError(r.key, err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case update, ok := <-updates:
			if !ok {
				return
			}
			if update.Message == nil || update.Message.Chat.ID != r.cfg.ChatId {
				continue
			}
			handleBoundCommand(r.bot, r.cfg, *update.Message)
		}
	}
}

func HandlePanelCommand(bot *telego.Bot, message telego.Message) bool {
	command, _, _ := tu.ParseCommand(message.Text)
	if command != "start" && command != "status" && command != "info" && command != "help" {
		return false
	}

	var cfg model.ClientTelegramBot
	err := database.GetDB().
		Where("enabled = ? AND mode = ? AND chat_id = ?", true, ModePanel, message.Chat.ID).
		First(&cfg).Error
	if err != nil {
		return false
	}

	handleBoundCommand(bot, cfg, message)
	return true
}

func handleBoundCommand(bot *telego.Bot, cfg model.ClientTelegramBot, message telego.Message) {
	command, _, _ := tu.ParseCommand(message.Text)
	switch command {
	case "start", "status", "info":
		_ = sendStatus(bot, cfg)
	case "help":
		_ = sendText(bot, cfg.ChatId,
			"可用命令：\n/status - 查看流量、到期、重置和连接信息\n/info - 查看当前客户端状态")
	}
}

func sendText(bot *telego.Bot, chatID int64, text string) error {
	if bot == nil {
		return fmt.Errorf("机器人未运行")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := bot.SendMessage(ctx, tu.Message(tu.ID(chatID), text))
	return err
}

func getClientSnapshot(cfg model.ClientTelegramBot) (*model.Inbound, *clientSnapshot, *xray.ClientTraffic, job.ClientActivityStat, error) {
	db := database.GetDB()
	if db == nil {
		return nil, nil, nil, job.ClientActivityStat{}, fmt.Errorf("数据库不可用")
	}

	inbound := &model.Inbound{}
	if err := db.First(inbound, cfg.InboundId).Error; err != nil {
		return nil, nil, nil, job.ClientActivityStat{}, err
	}

	var settings struct {
		Clients []clientSnapshot `json:"clients"`
	}
	if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
		return nil, nil, nil, job.ClientActivityStat{}, err
	}

	var client *clientSnapshot
	for i := range settings.Clients {
		if strings.EqualFold(strings.TrimSpace(settings.Clients[i].Email), strings.TrimSpace(cfg.Email)) {
			c := settings.Clients[i]
			client = &c
			break
		}
	}
	if client == nil {
		return nil, nil, nil, job.ClientActivityStat{}, fmt.Errorf("客户端不存在")
	}

	traffic := &xray.ClientTraffic{}
	err := db.Where("inbound_id = ? AND email = ?", cfg.InboundId, cfg.Email).First(traffic).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, nil, nil, job.ClientActivityStat{}, err
	}
	if err == gorm.ErrRecordNotFound {
		traffic = &xray.ClientTraffic{
			InboundId:  cfg.InboundId,
			Email:      cfg.Email,
			Total:      client.TotalGB,
			ExpiryTime: client.ExpiryTime,
			Reset:      client.Reset,
			Enable:     client.Enable,
		}
	}

	activity := job.ClientActivityStat{}
	all := job.GetInboundActivityStats()
	if in := all[cfg.InboundId]; in.Clients != nil {
		if v, ok := in.Clients[cfg.Email]; ok {
			activity = v
		}
	}
	return inbound, client, traffic, activity, nil
}

func formatRemaining(ms int64) string {
	if ms <= 0 {
		return "已到期"
	}
	d := time.Duration(ms) * time.Millisecond
	days := int(d / (24 * time.Hour))
	d -= time.Duration(days) * 24 * time.Hour
	hours := int(d / time.Hour)
	d -= time.Duration(hours) * time.Hour
	minutes := int(d / time.Minute)

	parts := make([]string, 0, 3)
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%d天", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%d小时", hours))
	}
	if minutes > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%d分钟", minutes))
	}
	return strings.Join(parts, " ")
}

func nextResetText(client *clientSnapshot, traffic *xray.ClientTraffic) string {
	reset := client.Reset
	expiry := client.ExpiryTime
	if traffic != nil {
		if traffic.Reset > 0 {
			reset = traffic.Reset
		}
		if traffic.ExpiryTime != 0 {
			expiry = traffic.ExpiryTime
		}
	}
	if reset <= 0 {
		return "未设置自动重置"
	}
	if expiry < 0 {
		return fmt.Sprintf("首次使用后每 %d 天重置", reset)
	}
	if expiry <= 0 {
		return fmt.Sprintf("每 %d 天重置（尚无下次时间）", reset)
	}
	return fmt.Sprintf("%s（剩余 %s）",
		time.UnixMilli(expiry).Format("2006-01-02 15:04:05"),
		formatRemaining(expiry-time.Now().UnixMilli()))
}

func buildStatusText(cfg model.ClientTelegramBot) (string, error) {
	inbound, client, traffic, activity, err := getClientSnapshot(cfg)
	if err != nil {
		return "", err
	}

	used := traffic.Up + traffic.Down
	total := client.TotalGB
	if traffic.Total > 0 {
		total = traffic.Total
	}

	remainingTraffic := "不限量"
	if total > 0 {
		remaining := total - used
		if remaining < 0 {
			remaining = 0
		}
		remainingTraffic = common.FormatTraffic(remaining)
	}

	expiry := client.ExpiryTime
	if traffic.ExpiryTime != 0 {
		expiry = traffic.ExpiryTime
	}
	expiryText := "永久"
	if expiry < 0 {
		expiryText = fmt.Sprintf("首次使用后 %d 天", expiry/-86400000)
	} else if expiry > 0 {
		expiryText = fmt.Sprintf("%s（%s）",
			time.UnixMilli(expiry).Format("2006-01-02 15:04:05"),
			formatRemaining(expiry-time.Now().UnixMilli()))
	}

	return fmt.Sprintf(
		"📊 客户端状态\n"+
			"📌 入站：%s\n"+
			"🔌 端口：%d\n"+
			"👤 用户：%s\n"+
			"⬆️ 上传：%s\n"+
			"⬇️ 下载：%s\n"+
			"📦 已用：%s\n"+
			"🎯 剩余：%s\n"+
			"⏰ 到期：%s\n"+
			"🔄 流量重置：%s\n"+
			"🌐 来源 IP：%d\n"+
			"🔗 当前连接：%d\n"+
			"🕒 查询时间：%s",
		inbound.Remark,
		inbound.Port,
		cfg.Email,
		common.FormatTraffic(traffic.Up),
		common.FormatTraffic(traffic.Down),
		common.FormatTraffic(used),
		remainingTraffic,
		expiryText,
		nextResetText(client, traffic),
		activity.SourceIPs,
		activity.OnlineConnections,
		time.Now().Format("2006-01-02 15:04:05"),
	), nil
}

func sendStatus(bot *telego.Bot, cfg model.ClientTelegramBot) error {
	text, err := buildStatusText(cfg)
	if err != nil {
		return sendText(bot, cfg.ChatId, "读取客户端状态失败："+compact(err.Error(), 180))
	}
	return sendText(bot, cfg.ChatId, text)
}

func GetConfig(inboundID int, email string) (ConfigView, error) {
	email = strings.TrimSpace(email)
	view := ConfigView{
		InboundID: inboundID,
		Email:     email,
		Mode:      ModePanel,
	}

	panelBot, panelUsername := panelBotInfo()
	view.PanelAvailable = panelBot != nil
	view.PanelBotUsername = panelUsername

	if inboundID <= 0 || email == "" {
		return view, nil
	}

	db := database.GetDB()
	var cfg model.ClientTelegramBot
	err := db.Where("inbound_id = ? AND email = ?", inboundID, email).First(&cfg).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return view, err
	}
	if err == nil {
		cfg.Mode = normalizeMode(cfg.Mode)
		view.Enabled = cfg.Enabled
		view.Mode = cfg.Mode
		view.ChatID = cfg.ChatId
		if cfg.Mode == ModeCustom {
			view.BotToken = cfg.BotToken
		}
	}

	if !view.Enabled {
		return view, nil
	}

	if view.Mode == ModePanel {
		view.Running = panelBot != nil
		view.BotUsername = panelUsername
		if panelBot == nil {
			view.LastError = "面板 Telegram 机器人未运行"
		}
		return view, nil
	}

	k := key(inboundID, email)
	manager.Lock()
	if r := manager.runners[k]; r != nil {
		view.Running = r.bot != nil && r.lastErr == ""
		view.BotUsername = r.username
		view.LastError = r.lastErr
	}
	manager.Unlock()
	return view, nil
}

func validatePanelBinding(inboundID int, email string, chatID int64) error {
	panelBot, _ := panelBotInfo()
	if panelBot == nil {
		return fmt.Errorf("面板 Telegram 机器人当前未启用或未运行")
	}
	if chatID == 0 {
		return fmt.Errorf("请填写 Telegram Chat ID")
	}

	var duplicate model.ClientTelegramBot
	err := database.GetDB().
		Where("enabled = ? AND mode = ? AND chat_id = ? AND NOT (inbound_id = ? AND email = ?)",
			true, ModePanel, chatID, inboundID, email).
		First(&duplicate).Error
	if err == nil {
		return fmt.Errorf("这个 Chat ID 已绑定其他客户端")
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}
	return nil
}

func validateCustomBinding(inboundID int, email, token string, chatID int64) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", fmt.Errorf("请填写 Bot Token")
	}
	if chatID == 0 {
		return "", fmt.Errorf("请填写 Telegram Chat ID")
	}

	var duplicate model.ClientTelegramBot
	err := database.GetDB().
		Where("enabled = ? AND mode = ? AND bot_token = ? AND NOT (inbound_id = ? AND email = ?)",
			true, ModeCustom, token, inboundID, email).
		First(&duplicate).Error
	if err == nil {
		return "", fmt.Errorf("同一个自定义 Bot Token 不能同时绑定多个客户端")
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return "", err
	}

	bot, err := telego.NewBot(token)
	if err != nil {
		return "", fmt.Errorf("机器人 Token 无效: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	me, err := bot.GetMe(ctx)
	if err != nil {
		return "", fmt.Errorf("机器人连接验证失败: %w", err)
	}
	return me.Username, nil
}

func SaveConfig(inboundID int, email string, enabled bool, mode, token string, chatID int64) (ConfigView, error) {
	email = strings.TrimSpace(email)
	mode = normalizeMode(mode)
	token = strings.TrimSpace(token)

	if inboundID <= 0 {
		return ConfigView{}, fmt.Errorf("入站 ID 无效")
	}
	if email == "" {
		return ConfigView{}, fmt.Errorf("请先填写并保存客户端电子邮件")
	}

	if enabled {
		if mode == ModePanel {
			if err := validatePanelBinding(inboundID, email, chatID); err != nil {
				return ConfigView{}, err
			}
			token = ""
		} else {
			if _, err := validateCustomBinding(inboundID, email, token, chatID); err != nil {
				return ConfigView{}, err
			}
		}
	}

	db := database.GetDB()
	var cfg model.ClientTelegramBot
	err := db.Where("inbound_id = ? AND email = ?", inboundID, email).First(&cfg).Error
	if err == gorm.ErrRecordNotFound {
		cfg = model.ClientTelegramBot{InboundId: inboundID, Email: email}
	} else if err != nil {
		return ConfigView{}, err
	}

	cfg.Enabled = enabled
	cfg.Mode = mode
	cfg.BotToken = token
	cfg.ChatId = chatID
	cfg.UpdatedAt = time.Now().Unix()
	if !enabled {
		cfg.ExpiryNotified = false
		cfg.TrafficNotified = false
	}

	if err := db.Save(&cfg).Error; err != nil {
		return ConfigView{}, err
	}
	Reconcile()
	return GetConfig(inboundID, email)
}

func Unbind(inboundID int, email string) error {
	email = strings.TrimSpace(email)
	if inboundID <= 0 || email == "" {
		return fmt.Errorf("客户端参数无效")
	}
	if err := database.GetDB().
		Where("inbound_id = ? AND email = ?", inboundID, email).
		Delete(&model.ClientTelegramBot{}).Error; err != nil {
		return err
	}
	Reconcile()
	return nil
}

func TestConfig(mode, token string, chatID int64) (string, error) {
	mode = normalizeMode(mode)
	if chatID == 0 {
		return "", fmt.Errorf("请填写 Telegram Chat ID")
	}

	if mode == ModePanel {
		bot, username := panelBotInfo()
		if bot == nil {
			return "", fmt.Errorf("面板 Telegram 机器人当前未启用或未运行")
		}
		if err := sendText(bot, chatID,
			"✅ DUI-PRO 客户端绑定测试成功\n来源：面板 Telegram\n机器人：@"+username); err != nil {
			return "", err
		}
		return username, nil
	}

	token = strings.TrimSpace(token)
	if token == "" {
		return "", fmt.Errorf("请填写 Bot Token")
	}
	bot, err := telego.NewBot(token)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	me, err := bot.GetMe(ctx)
	cancel()
	if err != nil {
		return "", err
	}
	if err := sendText(bot, chatID,
		"✅ DUI-PRO 客户端绑定测试成功\n来源：自定义 Telegram\n机器人：@"+me.Username); err != nil {
		return "", err
	}
	return me.Username, nil
}

type NotificationTarget struct {
	BindingID   int    `json:"bindingId"`
	InboundID   int    `json:"inboundId"`
	Protocol    string `json:"protocol"`
	Remark      string `json:"remark"`
	Port        int    `json:"port"`
	Email       string `json:"email"`
	Mode        string `json:"mode"`
	BotUsername string `json:"botUsername"`
	ChatID      int64  `json:"chatId"`
	Running     bool   `json:"running"`
	Label       string `json:"label"`
}

func ListNotificationTargets() ([]NotificationTarget, error) {
	db := database.GetDB()
	if db == nil {
		return nil, fmt.Errorf("数据库不可用")
	}

	var bindings []model.ClientTelegramBot
	if err := db.Where("enabled = ?", true).Order("inbound_id ASC, email ASC").Find(&bindings).Error; err != nil {
		return nil, err
	}
	if len(bindings) == 0 {
		return []NotificationTarget{}, nil
	}

	inboundIDs := make([]int, 0, len(bindings))
	seen := map[int]struct{}{}
	for _, binding := range bindings {
		if _, ok := seen[binding.InboundId]; !ok {
			seen[binding.InboundId] = struct{}{}
			inboundIDs = append(inboundIDs, binding.InboundId)
		}
	}

	var inbounds []model.Inbound
	if err := db.Where("id IN ?", inboundIDs).Find(&inbounds).Error; err != nil {
		return nil, err
	}
	inboundByID := make(map[int]model.Inbound, len(inbounds))
	for _, inbound := range inbounds {
		inboundByID[inbound.Id] = inbound
	}

	panelBot, panelUsername := panelBotInfo()
	result := make([]NotificationTarget, 0, len(bindings))
	for _, binding := range bindings {
		inbound, ok := inboundByID[binding.InboundId]
		if !ok {
			continue
		}
		mode := normalizeMode(binding.Mode)
		target := NotificationTarget{
			BindingID: binding.Id,
			InboundID: binding.InboundId,
			Protocol:  strings.ToUpper(string(inbound.Protocol)),
			Remark:    inbound.Remark,
			Port:      inbound.Port,
			Email:     binding.Email,
			Mode:      mode,
			ChatID:    binding.ChatId,
		}
		if mode == ModePanel {
			target.BotUsername = panelUsername
			target.Running = panelBot != nil
		} else {
			manager.Lock()
			r := manager.runners[key(binding.InboundId, binding.Email)]
			if r != nil {
				target.BotUsername = r.username
				target.Running = r.bot != nil && r.lastErr == ""
			}
			manager.Unlock()
		}
		tgLabel := "TG"
		if target.BotUsername != "" {
			tgLabel = "@" + target.BotUsername
		} else if mode == ModePanel {
			tgLabel = "面板TG"
		} else {
			tgLabel = "自定义TG"
		}
		target.Label = fmt.Sprintf("%s / %s - %d - %s - %s / %d",
			target.Protocol, target.Remark, target.Port, target.Email, tgLabel, target.ChatID)
		result = append(result, target)
	}
	return result, nil
}

func notificationBot(binding model.ClientTelegramBot) (*telego.Bot, string, error) {
	mode := normalizeMode(binding.Mode)
	if mode == ModePanel {
		bot, username := panelBotInfo()
		if bot == nil {
			return nil, username, fmt.Errorf("面板 Telegram 机器人当前未运行")
		}
		return bot, username, nil
	}

	manager.Lock()
	r := manager.runners[key(binding.InboundId, binding.Email)]
	manager.Unlock()
	if r == nil || r.bot == nil {
		Reconcile()
		manager.Lock()
		r = manager.runners[key(binding.InboundId, binding.Email)]
		manager.Unlock()
	}
	if r == nil || r.bot == nil {
		return nil, "", fmt.Errorf("客户端自定义 Telegram 机器人当前未运行")
	}
	if r.lastErr != "" {
		return nil, r.username, fmt.Errorf("%s", r.lastErr)
	}
	return r.bot, r.username, nil
}

func SendManualNotification(bindingID int, subject, content string) (*model.TelegramNotificationHistory, error) {
	subject = strings.TrimSpace(subject)
	content = strings.TrimSpace(content)
	if bindingID <= 0 {
		return nil, fmt.Errorf("请选择通知目标")
	}
	if subject == "" {
		return nil, fmt.Errorf("请填写通知主题")
	}
	if content == "" {
		return nil, fmt.Errorf("请填写通知内容")
	}
	if len([]rune(subject)) > 120 {
		return nil, fmt.Errorf("通知主题不能超过 120 个字符")
	}
	if len([]rune(content)) > 3500 {
		return nil, fmt.Errorf("通知内容不能超过 3500 个字符")
	}

	db := database.GetDB()
	var binding model.ClientTelegramBot
	if err := db.Where("id = ? AND enabled = ?", bindingID, true).First(&binding).Error; err != nil {
		return nil, fmt.Errorf("绑定目标不存在或已停用")
	}
	var inbound model.Inbound
	if err := db.First(&inbound, binding.InboundId).Error; err != nil {
		return nil, err
	}

	history := &model.TelegramNotificationHistory{
		CreatedAt: time.Now().Unix(),
		BindingId: binding.Id,
		InboundId: binding.InboundId,
		Protocol:  strings.ToUpper(string(inbound.Protocol)),
		Remark:    inbound.Remark,
		Port:      inbound.Port,
		Email:     binding.Email,
		Mode:      normalizeMode(binding.Mode),
		ChatId:    binding.ChatId,
		Subject:   subject,
		Content:   content,
	}

	sendBot, username, botErr := notificationBot(binding)
	history.BotUsername = username
	if botErr == nil {
		message := "📣 " + subject + "\n\n" + content
		botErr = sendText(sendBot, binding.ChatId, message)
	}
	if botErr != nil {
		history.Success = false
		history.Error = compact(botErr.Error(), 500)
	} else {
		history.Success = true
	}
	if err := db.Create(history).Error; err != nil {
		return history, err
	}
	if botErr != nil {
		return history, botErr
	}
	return history, nil
}

func ListNotificationHistory(limit int) ([]model.TelegramNotificationHistory, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	var rows []model.TelegramNotificationHistory
	err := database.GetDB().
		Order("created_at DESC, id DESC").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

func pollNotifications() {
	db := database.GetDB()
	if db == nil {
		return
	}
	var configs []model.ClientTelegramBot
	if err := db.Where("enabled = ?", true).Find(&configs).Error; err != nil {
		return
	}

	for i := range configs {
		cfg := &configs[i]
		cfg.Mode = normalizeMode(cfg.Mode)

		var sendBot *telego.Bot
		if cfg.Mode == ModePanel {
			sendBot, _ = panelBotInfo()
		} else {
			manager.Lock()
			r := manager.runners[key(cfg.InboundId, cfg.Email)]
			if r != nil && r.lastErr == "" {
				sendBot = r.bot
			}
			manager.Unlock()
		}
		if sendBot == nil {
			continue
		}

		_, client, traffic, _, err := getClientSnapshot(*cfg)
		if err != nil {
			continue
		}

		now := time.Now().UnixMilli()
		expiry := client.ExpiryTime
		if traffic.ExpiryTime != 0 {
			expiry = traffic.ExpiryTime
		}
		total := client.TotalGB
		if traffic.Total > 0 {
			total = traffic.Total
		}
		used := traffic.Up + traffic.Down
		expired := expiry > 0 && now >= expiry
		exhausted := total > 0 && used >= total

		changed := false
		if !expired && cfg.ExpiryNotified {
			cfg.ExpiryNotified = false
			changed = true
		}
		if !exhausted && cfg.TrafficNotified {
			cfg.TrafficNotified = false
			changed = true
		}
		if expired && !cfg.ExpiryNotified {
			_ = sendText(sendBot, cfg.ChatId,
				fmt.Sprintf("⏰ 客户端已到期\n👤 %s\n🕒 %s",
					cfg.Email, time.UnixMilli(expiry).Format("2006-01-02 15:04:05")))
			cfg.ExpiryNotified = true
			changed = true
		}
		if exhausted && !cfg.TrafficNotified {
			_ = sendText(sendBot, cfg.ChatId,
				fmt.Sprintf("📦 客户端流量已用完\n👤 %s\n📊 已用 %s / %s",
					cfg.Email, common.FormatTraffic(used), common.FormatTraffic(total)))
			cfg.TrafficNotified = true
			changed = true
		}
		if changed {
			cfg.UpdatedAt = time.Now().Unix()
			_ = db.Save(cfg).Error
		}
	}
}
