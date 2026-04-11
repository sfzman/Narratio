package handler

import "github.com/gin-gonic/gin"

func (h Handlers) healthCheck(c *gin.Context) {
	c.JSON(200, gin.H{
		"status":    "ok",
		"version":   h.health.Version,
		"services":  h.health.Services,
		"resources": h.health.Resources,
	})
}
