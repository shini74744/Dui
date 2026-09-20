package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func RedirectMiddleware(basePath string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Keep only the historical /xui alias and point it to the new /dui route.
		// The former route prefix is intentionally not registered or redirected.
		redirects := map[string]string{
			"xui/API": "dui/api",
			"xui":     "dui",
		}

		path := c.Request.URL.Path
		for from, to := range redirects {
			from, to = basePath+from, basePath+to

			if strings.HasPrefix(path, from) {
				newPath := to + path[len(from):]

				c.Redirect(http.StatusMovedPermanently, newPath)
				c.Abort()
				return
			}
		}

		c.Next()
	}
}
