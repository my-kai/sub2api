// Package routes registers the prompt-audit-v2 custom management namespace.
package routes

import (
	promptauditv2 "github.com/Wei-Shaw/sub2api/internal/custom/promptauditv2"
	"github.com/gin-gonic/gin"
)

// RegisterAdminRoutes attaches system-administrator endpoints below /admin.
func RegisterAdminRoutes(admin *gin.RouterGroup, handler *promptauditv2.AdminHandler) {
	if admin == nil || handler == nil {
		return
	}
	group := admin.Group("/custom/prompt-audit-v2")
	group.GET("/config", handler.GetConfig)
	group.PUT("/config", handler.UpdateConfig)
	group.POST("/endpoints/probe", handler.ProbeEndpoint)
	group.POST("/prompt/test", handler.TestPrompt)
	group.GET("/runtime", handler.GetRuntime)
	group.GET("/events", handler.ListEvents)
	group.GET("/events/:id", handler.GetEvent)
}
