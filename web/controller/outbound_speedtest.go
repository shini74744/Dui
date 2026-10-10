package controller

import (
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/url"
	"x-ui/web/service"
)

func speedTestSameOrigin(c *gin.Context) bool {
	if c.GetHeader("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	if origin := c.GetHeader("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Host != c.Request.Host {
			return false
		}
	}
	return true
}
func (a *XraySettingController) speedTestStatus(c *gin.Context) {
	jsonObj(c, service.OutboundSpeedTests.Snapshot(), nil)
}
func (a *XraySettingController) speedTestCatalog(c *gin.Context) {
	raw, err := a.SettingService.GetXrayConfigTemplate()
	if err != nil {
		jsonObj(c, nil, errors.New("config_unavailable"))
		return
	}
	jsonObj(c, service.SpeedTestCatalog([]byte(raw)), nil)
}
func (a *XraySettingController) speedTestStart(c *gin.Context) {
	if !speedTestSameOrigin(c) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8192)
	req, err := bindSpeedTestRequest(c)
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	raw, err := a.SettingService.GetXrayConfigTemplate()
	if err != nil {
		jsonObj(c, nil, errors.New("config_unavailable"))
		return
	}
	job, err := service.OutboundSpeedTests.Start([]byte(raw), req)
	jsonObj(c, job, err)
}
func (a *XraySettingController) speedTestCancel(c *gin.Context) {
	if !speedTestSameOrigin(c) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	var req struct {
		ID string `json:"id" form:"id"`
	}
	if c.ShouldBind(&req) != nil {
		jsonObj(c, nil, errors.New("invalid_request"))
		return
	}
	err := service.OutboundSpeedTests.Cancel(req.ID)
	jsonObj(c, service.OutboundSpeedTests.Snapshot(), err)
}

func bindSpeedTestRequest(c *gin.Context) (service.SpeedTestRequest, error) {
	var req service.SpeedTestRequest
	// HttpUtil sends a form; also accept JSON for API clients.
	if c.ContentType() == "application/json" {
		var body struct {
			Tags    []string `json:"tags"`
			Threads int      `json:"threads"`
			Request string   `json:"request"`
		}
		if c.ShouldBindJSON(&body) != nil {
			return req, errors.New("invalid_request")
		}
		req.Tags, req.Threads = body.Tags, body.Threads
		if body.Request != "" && json.Unmarshal([]byte(body.Request), &req) != nil {
			return req, errors.New("invalid_request")
		}
	} else {
		payload := c.PostForm("request")
		if json.Unmarshal([]byte(payload), &req) != nil {
			return req, errors.New("invalid_request")
		}
	}

	return req, nil
}
