package middleware

import (
	"github.com/gin-gonic/gin"
	"strings"
	"x-ui/database"
)

// ImportDB takes the exclusive lock only after validating its staged upload.
func DatabaseGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasSuffix(c.FullPath(), "/importDB") {
			c.Next()
			return
		}
		database.MaintenanceMu.RLock()
		defer database.MaintenanceMu.RUnlock()
		c.Next()
	}
}
