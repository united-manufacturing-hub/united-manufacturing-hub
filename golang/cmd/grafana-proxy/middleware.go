package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
)

func AddCorsHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := internal.SanitizeString(c.GetHeader("Origin"))
		zap.S().Debugf("Requesting origin: %s", origin)
		if len(origin) == 0 {
			zap.S().Debugf("Add cors wildcard")
			origin = "*"
		} else {
			zap.S().Debugf("Set cors origin to: %s", origin)
		}
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Methods", "*")

		if c.Request.Method == "OPTIONS" {
			zap.S().Debugf("optionsCORSHandler")
			c.AbortWithStatus(http.StatusOK)
		}
	}
}
