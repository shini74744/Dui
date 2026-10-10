package controller

import (
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestSpeedTestRequestFormsAndJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct{ contentType, body string }{
		{"application/x-www-form-urlencoded", url.Values{"request": {`{"tags":["node-a"],"threads":8}`}}.Encode()},
		{"application/json", `{"tags":["node-a"],"threads":8}`},
		{"application/json", `{"request":"{\"tags\":[\"node-a\"],\"threads\":8}"}`},
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "https://panel.test/dui/xray/speedTest/start", strings.NewReader(tc.body))
		c.Request.Header.Set("Content-Type", tc.contentType)
		req, err := bindSpeedTestRequest(c)
		if err != nil || req.Threads != 8 || len(req.Tags) != 1 || req.Tags[0] != "node-a" {
			t.Fatalf("format %s: %+v %v", tc.contentType, req, err)
		}
	}
}
func TestSpeedTestOriginGuard(t *testing.T) {
	for _, tc := range []struct {
		origin, site string
		want         bool
	}{
		{"https://panel.test:744", "same-origin", true},
		{"https://other.test", "cross-site", false},
		{"https://other.test", "", false},
		{"null", "", false},
		{"", "", true},
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "https://panel.test:744/dui/xray/speedTest/start", nil)
		c.Request.Header.Set("Origin", tc.origin)
		c.Request.Header.Set("Sec-Fetch-Site", tc.site)
		if speedTestSameOrigin(c) != tc.want {
			t.Fatalf("origin %q", tc.origin)
		}
	}
}
