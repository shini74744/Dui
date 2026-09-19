package controller

import (
	"fmt"

	"x-ui/web/service"

	"github.com/gin-gonic/gin"
)

type XraySettingController struct {
	XraySettingService service.XraySettingService
	SettingService     service.SettingService
	InboundService     service.InboundService
	OutboundService    service.OutboundService
	XrayService        service.XrayService
	WarpService        service.WarpService
	RuleSetService     service.RuleSetService
}

func NewXraySettingController(g *gin.RouterGroup) *XraySettingController {
	a := &XraySettingController{}
	a.initRouter(g)
	return a
}

func (a *XraySettingController) initRouter(g *gin.RouterGroup) {
	g = g.Group("/xray")

	g.POST("/", a.getXraySetting)
	g.POST("/update", a.updateSetting)
	g.GET("/getXrayResult", a.getXrayResult)
	g.GET("/getDefaultJsonConfig", a.getDefaultXrayConfig)
	g.POST("/warp/:action", a.warp)
	g.GET("/getOutboundsTraffic", a.getOutboundsTraffic)
	g.POST("/resetOutboundsTraffic", a.resetOutboundsTraffic)
	g.POST("/blacklist/list", a.getManagedBlacklist)
	g.POST("/blacklist/remove", a.removeManagedBlacklist)
	g.POST("/blacklist/clear", a.clearManagedBlacklist)
	g.GET("/ruleSet/apps", a.getRuleSetApps)
	g.POST("/ruleSet/resolve", a.resolveRuleSet)
	g.POST("/ruleSet/resolveURL", a.resolveRuleSetURL)
	g.POST("/ruleSet/resolveMany", a.resolveRuleSetMany)
}

func (a *XraySettingController) getXraySetting(c *gin.Context) {
	xraySetting, err := a.SettingService.GetXrayConfigTemplate()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.getSettings"), err)
		return
	}
	inboundTags, err := a.InboundService.GetInboundTags()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.getSettings"), err)
		return
	}
	xrayResponse := "{ \"xraySetting\": " + xraySetting + ", \"inboundTags\": " + inboundTags + " }"
	jsonObj(c, xrayResponse, nil)
}

func (a *XraySettingController) updateSetting(c *gin.Context) {
	xraySetting := c.PostForm("xraySetting")
	oldSetting, oldErr := a.SettingService.GetXrayConfigTemplate()
	if oldErr != nil {
		jsonMsg(c, "读取旧 Xray 配置失败，未保存新配置", oldErr)
		return
	}

	if err := a.XraySettingService.SaveXraySetting(xraySetting); err != nil {
		jsonMsg(c, "Xray 配置校验失败，未保存", err)
		return
	}

	if err := a.XrayService.RestartXray(true); err != nil {
		rollbackErr := a.XraySettingService.SaveXraySetting(oldSetting)
		if rollbackErr == nil {
			rollbackErr = a.XrayService.RestartXray(true)
		}
		if rollbackErr != nil {
			jsonMsg(c, "新 Xray 配置启动失败；自动回滚也失败，请立即检查 Xray 日志", fmt.Errorf("新配置错误: %v；回滚错误: %v", err, rollbackErr))
			return
		}
		jsonMsg(c, "新 Xray 配置启动失败，已自动恢复旧配置", err)
		return
	}

	jsonMsg(c, "Xray 配置已保存并自动重载，路由规则已生效", nil)
}

func (a *XraySettingController) getDefaultXrayConfig(c *gin.Context) {
	defaultJsonConfig, err := a.SettingService.GetDefaultXrayConfig()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.getSettings"), err)
		return
	}
	jsonObj(c, defaultJsonConfig, nil)
}

func (a *XraySettingController) getXrayResult(c *gin.Context) {
	jsonObj(c, a.XrayService.GetXrayResult(), nil)
}

func (a *XraySettingController) warp(c *gin.Context) {
	action := c.Param("action")
	var resp string
	var err error
	switch action {
	case "data":
		resp, err = a.WarpService.GetWarpData()
	case "del":
		err = a.WarpService.DelWarpData()
	case "config":
		resp, err = a.WarpService.GetWarpConfig()
	case "reg":
		skey := c.PostForm("privateKey")
		pkey := c.PostForm("publicKey")
		resp, err = a.WarpService.RegWarp(skey, pkey)
	case "license":
		license := c.PostForm("license")
		resp, err = a.WarpService.SetWarpLicense(license)
	}

	jsonObj(c, resp, err)
}

func (a *XraySettingController) getOutboundsTraffic(c *gin.Context) {
	outboundsTraffic, err := a.OutboundService.GetOutboundsTraffic()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.getOutboundTrafficError"), err)
		return
	}
	jsonObj(c, outboundsTraffic, nil)
}

func (a *XraySettingController) resetOutboundsTraffic(c *gin.Context) {
	tag := c.PostForm("tag")
	err := a.OutboundService.ResetOutboundTraffic(tag)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.resetOutboundTrafficError"), err)
		return
	}
	jsonObj(c, "", nil)
}

type managedBlacklistRemoveForm struct {
	ID int `json:"id" form:"id"`
}

type managedBlacklistClearForm struct {
	Kind string `json:"kind" form:"kind"`
}

func (a *XraySettingController) getManagedBlacklist(c *gin.Context) {
	svc := &service.ManagedBlacklistService{}
	rows, err := svc.List()
	jsonObj(c, rows, err)
}

func (a *XraySettingController) removeManagedBlacklist(c *gin.Context) {
	form := &managedBlacklistRemoveForm{}
	if err := c.ShouldBind(form); err != nil {
		jsonMsg(c, "黑名单参数无效", err)
		return
	}
	svc := &service.ManagedBlacklistService{}
	err := svc.RemoveByID(form.ID)
	jsonObj(c, map[string]any{"removed": err == nil}, err)
}

func (a *XraySettingController) clearManagedBlacklist(c *gin.Context) {
	form := &managedBlacklistClearForm{}
	_ = c.ShouldBind(form)
	svc := &service.ManagedBlacklistService{}
	err := svc.Clear(form.Kind)
	jsonObj(c, map[string]any{"cleared": err == nil}, err)
}

type ruleSetResolveForm struct {
	Path string `json:"path" form:"path"`
}

type ruleSetResolveURLForm struct {
	URL string `json:"url" form:"url"`
}

type ruleSetResolveManyForm struct {
	Paths []string `json:"paths" form:"paths"`
}

func (a *XraySettingController) getRuleSetApps(c *gin.Context) {
	force := c.Query("refresh") == "1" || c.Query("refresh") == "true"
	apps, err := a.RuleSetService.Apps(force)
	jsonObj(c, apps, err)
}

func (a *XraySettingController) resolveRuleSet(c *gin.Context) {
	form := &ruleSetResolveForm{}
	if err := c.ShouldBind(form); err != nil {
		jsonMsg(c, "第三方规则参数无效", err)
		return
	}
	result, err := a.RuleSetService.Resolve(form.Path)
	jsonObj(c, result, err)
}

func (a *XraySettingController) resolveRuleSetURL(c *gin.Context) {
	form := &ruleSetResolveURLForm{}
	if err := c.ShouldBind(form); err != nil {
		jsonMsg(c, "自定义 List 地址参数无效", err)
		return
	}
	result, err := a.RuleSetService.ResolveURL(form.URL)
	jsonObj(c, result, err)
}

func (a *XraySettingController) resolveRuleSetMany(c *gin.Context) {
	form := &ruleSetResolveManyForm{}
	if err := c.ShouldBind(form); err != nil {
		jsonMsg(c, "第三方分类规则参数无效", err)
		return
	}
	result, err := a.RuleSetService.ResolveMany(form.Paths)
	jsonObj(c, result, err)
}
