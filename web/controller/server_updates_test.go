package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"x-ui/database/model"
	"x-ui/web/session"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

func updateTestRouter(loggedIn bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(sessions.Sessions("dui-test", cookie.NewStore([]byte("updater-test-session-key-32-bytes!"))))
	r.Use(func(c *gin.Context) {
		if loggedIn {
			session.SetLoginUser(c, &model.User{})
		}
		c.Next()
	})
	a := &ServerController{}
	a.initRouter(r.Group("/dui/api/server"))
	return r
}

func TestUpdaterStartAcceptsPageFormAndJSON(t *testing.T) {
	for _, tc := range []struct{ name, contentType, body string }{
		{"page-form", "application/x-www-form-urlencoded; charset=UTF-8", "kind=panel&version=v0.0.1"},
		{"json-client", "application/json", `{"kind":"panel","version":"v0.0.1"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/dui/api/server/updates/start", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.contentType)
			req.Header.Set("Origin", "http://example.com")
			res := httptest.NewRecorder()
			updateTestRouter(true).ServeHTTP(res, req)
			var result struct {
				Success bool
				Msg     string
			}
			if err := json.Unmarshal(res.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			// Reaching this guard proves both submitted fields were parsed by the real route.
			// The old version avoids filesystem changes or starting any update worker.
			if res.Code != http.StatusOK || result.Success || !strings.Contains(result.Msg, "already_current") {
				t.Fatalf("request did not reach version validation: status=%d response=%s", res.Code, res.Body.String())
			}
		})
	}
}

func TestUpdaterStartPreservesRequestGuards(t *testing.T) {
	for _, tc := range []struct {
		name, contentType, body, origin, fetchSite string
		loggedIn                                   bool
		status                                     int
	}{
		{"form-needs-login", "application/x-www-form-urlencoded", "kind=panel&version=v0.0.1", "", "", false, http.StatusUnauthorized},
		{"form-cross-origin", "application/x-www-form-urlencoded", "kind=panel&version=v0.0.1", "https://elsewhere.invalid", "", true, http.StatusForbidden},
		{"form-cross-site", "application/x-www-form-urlencoded", "kind=panel&version=v0.0.1", "", "cross-site", true, http.StatusForbidden},
		{"json-cross-origin", "application/json", `{"kind":"panel","version":"v0.0.1"}`, "https://elsewhere.invalid", "", true, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/dui/api/server/updates/start", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.contentType)
			req.Header.Set("X-Requested-With", "XMLHttpRequest")
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("Sec-Fetch-Site", tc.fetchSite)
			res := httptest.NewRecorder()
			updateTestRouter(tc.loggedIn).ServeHTTP(res, req)
			if res.Code != tc.status {
				t.Fatalf("got %d, want %d", res.Code, tc.status)
			}
		})
	}
}
