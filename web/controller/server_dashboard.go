package controller

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

type networkExitPolicyForm struct {
	Family string `json:"family" form:"family"`
	Action string `json:"action" form:"action"`
}

func (a *ServerController) dailyTrafficRanking(c *gin.Context) {
	limit := 5
	if raw := c.Query("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	rows, err := a.serverService.GetDailyTrafficRanking(limit)
	jsonObj(c, rows, err)
}

func (a *ServerController) getNetworkExitPolicy(c *gin.Context) {
	jsonObj(c, a.serverService.GetNetworkExitPolicy(), nil)
}
func (a *ServerController) setNetworkExitPolicy(c *gin.Context) {
	form := &networkExitPolicyForm{}
	if err := c.ShouldBind(form); err != nil {
		jsonMsg(c, "出口网络参数无效", err)
		return
	}
	policy, err := a.serverService.SetNetworkExitPolicy(form.Family, form.Action)
	if err != nil {
		jsonObj(c, policy, err)
		return
	}
	jsonObj(c, policy, nil)
}
