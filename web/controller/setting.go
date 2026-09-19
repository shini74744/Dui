package controller

import (
	"errors"
	"time"

	"x-ui/util/crypto"
	"x-ui/web/clientbot"
	"x-ui/web/entity"
	"x-ui/web/service"
	"x-ui/web/session"

	"github.com/gin-gonic/gin"
)

type updateUserForm struct {
	OldUsername string `json:"oldUsername" form:"oldUsername"`
	OldPassword string `json:"oldPassword" form:"oldPassword"`
	NewUsername string `json:"newUsername" form:"newUsername"`
	NewPassword string `json:"newPassword" form:"newPassword"`
}

type SettingController struct {
	settingService     service.SettingService
	userService        service.UserService
	panelService       service.PanelService
	fail2banService    service.Fail2banService
	timeSyncService    service.TimeSyncService
	ddnsService        service.DDNSService
	certManagerService service.CertManagerService
}

type fail2banApplyForm struct {
	Target   string `json:"target" form:"target"`
	Port     int    `json:"port" form:"port"`
	MaxRetry int    `json:"maxretry" form:"maxretry"`
	FindTime string `json:"findtime" form:"findtime"`
	BanTime  string `json:"bantime" form:"bantime"`
	IgnoreIP string `json:"ignoreip" form:"ignoreip"`
}

type fail2banUnbanForm struct {
	Jail string `json:"jail" form:"jail"`
	IP   string `json:"ip" form:"ip"`
}

type fail2banRemoveForm struct {
	Target string `json:"target" form:"target"`
}

type timeSyncTimezoneForm struct {
	Timezone string `json:"timezone" form:"timezone"`
}

type timeSyncResolveForm struct {
	Query string `json:"query" form:"query"`
}

type ddnsAddForm struct {
	Zone          string `json:"zone" form:"zone"`
	Name          string `json:"name" form:"name"`
	Token         string `json:"token" form:"token"`
	CreateMissing bool   `json:"createMissing" form:"createMissing"`
}

type ddnsNameForm struct {
	Name string `json:"name" form:"name"`
}

type ddnsEnabledForm struct {
	Name    string `json:"name" form:"name"`
	Enabled bool   `json:"enabled" form:"enabled"`
}

type ddnsTokenForm struct {
	Name  string `json:"name" form:"name"`
	Token string `json:"token" form:"token"`
}

type ddnsIntervalForm struct {
	Interval int `json:"interval" form:"interval"`
}

type ddnsScheduleForm struct {
	Enabled bool `json:"enabled" form:"enabled"`
}

type certManagerUpdateForm struct {
	Enabled bool `json:"enabled" form:"enabled"`
	Months  int  `json:"months" form:"months"`
}

type telegramNotificationSendForm struct {
	BindingID int    `json:"bindingId" form:"bindingId"`
	Subject   string `json:"subject" form:"subject"`
	Content   string `json:"content" form:"content"`
}

type telegramNotificationHistoryForm struct {
	Limit int `json:"limit" form:"limit"`
}

func NewSettingController(g *gin.RouterGroup) *SettingController {
	a := &SettingController{}
	a.initRouter(g)
	return a
}

func (a *SettingController) initRouter(g *gin.RouterGroup) {
	g = g.Group("/setting")

	g.POST("/all", a.getAllSetting)
	g.POST("/defaultSettings", a.getDefaultSettings)
	g.POST("/update", a.updateSetting)
	g.POST("/updateUser", a.updateUser)
	g.POST("/restartPanel", a.restartPanel)
	g.GET("/getDefaultJsonConfig", a.getDefaultXrayConfig)
	g.POST("/fail2ban/status", a.getFail2banStatus)
	g.POST("/fail2ban/apply", a.applyFail2ban)
	g.POST("/fail2ban/unban", a.unbanFail2ban)
	g.POST("/fail2ban/remove", a.removeFail2ban)
	g.POST("/fail2ban/syncScript", a.syncFail2banScript)

	g.POST("/timeSync/status", a.getTimeSyncStatus)
	g.POST("/timeSync/timezones", a.getTimezones)
	g.POST("/timeSync/resolve", a.resolveTimezones)
	g.POST("/timeSync/install", a.installTimeSync)
	g.POST("/timeSync/sync", a.syncTimeNow)
	g.POST("/timeSync/timezone", a.setTimeSyncTimezone)
	g.POST("/timeSync/logs", a.getTimeSyncLogs)
	g.POST("/timeSync/uninstall", a.uninstallTimeSync)

	g.POST("/ddns/status", a.getDDNSStatus)
	g.POST("/ddns/install", a.installDDNS)
	g.POST("/ddns/add", a.addDDNSRecord)
	g.POST("/ddns/delete", a.deleteDDNSRecord)
	g.POST("/ddns/enabled", a.setDDNSRecordEnabled)
	g.POST("/ddns/token", a.replaceDDNSToken)
	g.POST("/ddns/interval", a.setDDNSInterval)
	g.POST("/ddns/schedule", a.setDDNSSchedule)
	g.POST("/ddns/run", a.runDDNSNow)
	g.POST("/ddns/logs", a.getDDNSLogs)
	g.POST("/ddns/updateScript", a.updateDDNSScript)
	g.POST("/ddns/uninstall", a.uninstallDDNS)

	g.POST("/certManager/status", a.getCertManagerStatus)
	g.POST("/certManager/update", a.updateCertManager)
	g.POST("/certManager/renew", a.renewCertManagerNow)

	g.POST("/telegram/notification/targets", a.getTelegramNotificationTargets)
	g.POST("/telegram/notification/send", a.sendTelegramNotification)
	g.POST("/telegram/notification/history", a.getTelegramNotificationHistory)
}

func (a *SettingController) getAllSetting(c *gin.Context) {
	allSetting, err := a.settingService.GetAllSetting()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.getSettings"), err)
		return
	}
	jsonObj(c, allSetting, nil)
}

func (a *SettingController) getTelegramNotificationTargets(c *gin.Context) {
	targets, err := clientbot.ListNotificationTargets()
	jsonObj(c, targets, err)
}

func (a *SettingController) sendTelegramNotification(c *gin.Context) {
	form := &telegramNotificationSendForm{}
	if err := c.ShouldBind(form); err != nil {
		jsonMsg(c, "通知参数无效", err)
		return
	}
	history, err := clientbot.SendManualNotification(form.BindingID, form.Subject, form.Content)
	jsonObj(c, history, err)
}

func (a *SettingController) getTelegramNotificationHistory(c *gin.Context) {
	form := &telegramNotificationHistoryForm{}
	_ = c.ShouldBind(form)
	rows, err := clientbot.ListNotificationHistory(form.Limit)
	jsonObj(c, rows, err)
}

func (a *SettingController) getDefaultSettings(c *gin.Context) {
	result, err := a.settingService.GetDefaultSettings(c.Request.Host)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.getSettings"), err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *SettingController) updateSetting(c *gin.Context) {
	allSetting := &entity.AllSetting{}
	err := c.ShouldBind(allSetting)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), err)
		return
	}
	err = a.settingService.UpdateAllSetting(allSetting)
	jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), err)
}

func (a *SettingController) updateUser(c *gin.Context) {
	form := &updateUserForm{}
	err := c.ShouldBind(form)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), err)
		return
	}
	user := session.GetLoginUser(c)
	if user.Username != form.OldUsername || !crypto.CheckPasswordHash(user.Password, form.OldPassword) {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifyUserError"), errors.New(I18nWeb(c, "pages.settings.toasts.originalUserPassIncorrect")))
		return
	}
	if form.NewUsername == "" || form.NewPassword == "" {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifyUserError"), errors.New(I18nWeb(c, "pages.settings.toasts.userPassMustBeNotEmpty")))
		return
	}
	err = a.userService.UpdateUser(user.Id, form.NewUsername, form.NewPassword)
	if err == nil {
		user.Username = form.NewUsername
		user.Password, _ = crypto.HashPasswordAsBcrypt(form.NewPassword)
		session.SetLoginUser(c, user)
	}
	jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifyUser"), err)
}

func (a *SettingController) restartPanel(c *gin.Context) {
	err := a.panelService.RestartPanel(time.Second * 3)
	jsonMsg(c, I18nWeb(c, "pages.settings.restartPanelSuccess"), err)
}

func (a *SettingController) getDefaultXrayConfig(c *gin.Context) {
	defaultJsonConfig, err := a.settingService.GetDefaultXrayConfig()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.getSettings"), err)
		return
	}
	jsonObj(c, defaultJsonConfig, nil)
}

func (a *SettingController) getFail2banStatus(c *gin.Context) {
	status, err := a.fail2banService.Status()
	jsonObj(c, status, err)
}

func (a *SettingController) applyFail2ban(c *gin.Context) {
	form := &fail2banApplyForm{}
	if err := c.ShouldBind(form); err != nil {
		jsonMsg(c, "Fail2ban 参数无效", err)
		return
	}
	err := a.fail2banService.Apply(service.Fail2banApplyOptions{
		Target: form.Target, Port: form.Port, MaxRetry: form.MaxRetry,
		FindTime: form.FindTime, BanTime: form.BanTime, IgnoreIP: form.IgnoreIP,
	})
	jsonMsg(c, "Fail2ban 配置已应用", err)
}

func (a *SettingController) unbanFail2ban(c *gin.Context) {
	form := &fail2banUnbanForm{}
	if err := c.ShouldBind(form); err != nil {
		jsonMsg(c, "解禁参数无效", err)
		return
	}
	jsonMsg(c, "IP 已解禁", a.fail2banService.Unban(form.Jail, form.IP))
}

func (a *SettingController) removeFail2ban(c *gin.Context) {
	form := &fail2banRemoveForm{}
	if err := c.ShouldBind(form); err != nil {
		jsonMsg(c, "删除参数无效", err)
		return
	}
	jsonMsg(c, "Fail2ban 配置已删除", a.fail2banService.Remove(form.Target))
}

func (a *SettingController) syncFail2banScript(c *gin.Context) {
	jsonMsg(c, "已同步项目内置 fb5 到 /usr/local/bin/fb5", a.fail2banService.SyncScript())
}

func (a *SettingController) getTimeSyncStatus(c *gin.Context) {
	status, err := a.timeSyncService.Status()
	jsonObj(c, status, err)
}

func (a *SettingController) getTimezones(c *gin.Context) {
	zones, err := a.timeSyncService.Timezones()
	jsonObj(c, zones, err)
}

func (a *SettingController) resolveTimezones(c *gin.Context) {
	form := &timeSyncResolveForm{}
	if err := c.ShouldBind(form); err != nil {
		jsonMsg(c, "地区/时区参数无效", err)
		return
	}
	zones, err := a.timeSyncService.ResolveTimezones(form.Query)
	jsonObj(c, zones, err)
}

func (a *SettingController) installTimeSync(c *gin.Context) {
	form := &timeSyncTimezoneForm{}
	if err := c.ShouldBind(form); err != nil {
		jsonMsg(c, "时区参数无效", err)
		return
	}
	out, err := a.timeSyncService.Install(form.Timezone)
	service.NotifyTimeSyncAction("安装/更新时间同步", err)
	jsonMsgObj(c, "时间同步管理器已安装/更新", out, err)
}

func (a *SettingController) syncTimeNow(c *gin.Context) {
	out, err := a.timeSyncService.SyncNow()
	service.NotifyTimeSyncAction("手动立即同步", err)
	jsonMsgObj(c, "时间同步已执行", out, err)
}

func (a *SettingController) setTimeSyncTimezone(c *gin.Context) {
	form := &timeSyncTimezoneForm{}
	if err := c.ShouldBind(form); err != nil {
		jsonMsg(c, "时区参数无效", err)
		return
	}
	out, err := a.timeSyncService.SetTimezone(form.Timezone)
	service.NotifyTimeSyncAction("修改时区："+form.Timezone, err)
	jsonMsgObj(c, "时区已修改", out, err)
}

func (a *SettingController) getTimeSyncLogs(c *gin.Context) {
	logs, err := a.timeSyncService.Logs()
	jsonObj(c, logs, err)
}

func (a *SettingController) uninstallTimeSync(c *gin.Context) {
	jsonMsg(c, "时间同步定时任务已卸载，systemd-timesyncd 保留", a.timeSyncService.Uninstall())
}

func (a *SettingController) getDDNSStatus(c *gin.Context) {
	status, err := a.ddnsService.Status()
	jsonObj(c, status, err)
}

func (a *SettingController) installDDNS(c *gin.Context) {
	out, err := a.ddnsService.Install()
	jsonMsgObj(c, "DDNS 已安装/更新", out, err)
}

func (a *SettingController) addDDNSRecord(c *gin.Context) {
	form := &ddnsAddForm{}
	if err := c.ShouldBind(form); err != nil {
		jsonMsg(c, "DDNS 参数无效", err)
		return
	}
	out, err := a.ddnsService.Add(form.Zone, form.Name, form.Token, form.CreateMissing)
	jsonMsgObj(c, "域名已加入 DDNS", out, err)
}

func (a *SettingController) deleteDDNSRecord(c *gin.Context) {
	form := &ddnsNameForm{}
	if err := c.ShouldBind(form); err != nil {
		jsonMsg(c, "域名参数无效", err)
		return
	}
	out, err := a.ddnsService.Delete(form.Name)
	jsonMsgObj(c, "已从本机 DDNS 管理移除", out, err)
}

func (a *SettingController) setDDNSRecordEnabled(c *gin.Context) {
	form := &ddnsEnabledForm{}
	if err := c.ShouldBind(form); err != nil {
		jsonMsg(c, "域名状态参数无效", err)
		return
	}
	out, err := a.ddnsService.SetEnabled(form.Name, form.Enabled)
	jsonMsgObj(c, "域名启用状态已修改", out, err)
}

func (a *SettingController) replaceDDNSToken(c *gin.Context) {
	form := &ddnsTokenForm{}
	if err := c.ShouldBind(form); err != nil {
		jsonMsg(c, "Token 参数无效", err)
		return
	}
	out, err := a.ddnsService.ReplaceToken(form.Name, form.Token)
	jsonMsgObj(c, "Cloudflare Token 已更新", out, err)
}

func (a *SettingController) setDDNSInterval(c *gin.Context) {
	form := &ddnsIntervalForm{}
	if err := c.ShouldBind(form); err != nil {
		jsonMsg(c, "间隔参数无效", err)
		return
	}
	out, err := a.ddnsService.SetInterval(form.Interval)
	jsonMsgObj(c, "DDNS 检查间隔已修改", out, err)
}

func (a *SettingController) setDDNSSchedule(c *gin.Context) {
	form := &ddnsScheduleForm{}
	if err := c.ShouldBind(form); err != nil {
		jsonMsg(c, "调度参数无效", err)
		return
	}
	out, err := a.ddnsService.SetSchedule(form.Enabled)
	jsonMsgObj(c, "DDNS 自动同步状态已修改", out, err)
}

func (a *SettingController) runDDNSNow(c *gin.Context) {
	out, err := a.ddnsService.RunNow()
	service.NotifyDDNSAction("手动检测并同步", err)
	jsonMsgObj(c, "DDNS 手动检测已完成", out, err)
}

func (a *SettingController) getDDNSLogs(c *gin.Context) {
	jsonObj(c, a.ddnsService.Logs(), nil)
}

func (a *SettingController) updateDDNSScript(c *gin.Context) {
	out, err := a.ddnsService.UpdateScript()
	jsonMsgObj(c, "DDNS 在线更新已执行", out, err)
}

func (a *SettingController) uninstallDDNS(c *gin.Context) {
	out, err := a.ddnsService.Uninstall()
	jsonMsgObj(c, "本机 DDNS 已卸载，Cloudflare DNS 记录未删除", out, err)
}

func (a *SettingController) getCertManagerStatus(c *gin.Context) {
	status, err := a.certManagerService.Status()
	jsonObj(c, status, err)
}

func (a *SettingController) updateCertManager(c *gin.Context) {
	form := &certManagerUpdateForm{}
	if err := c.ShouldBind(form); err != nil {
		jsonMsg(c, "证书续期参数无效", err)
		return
	}
	err := a.certManagerService.Update(form.Enabled, form.Months)
	jsonMsg(c, "证书强制更新设置已保存", err)
}

func (a *SettingController) renewCertManagerNow(c *gin.Context) {
	out, err := a.certManagerService.ForceRenewNow()
	jsonMsgObj(c, "当前证书已执行强制更新", out, err)
}
