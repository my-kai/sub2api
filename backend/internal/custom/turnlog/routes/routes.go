package routes

import (
	turnlog "github.com/Wei-Shaw/sub2api/internal/custom/turnlog"
	"github.com/gin-gonic/gin"
)

// RegisterAdminRoutes registers the system-administrator turn-log APIs.
func RegisterAdminRoutes(group gin.IRouter, handler *turnlog.Handler) {
	if group == nil || handler == nil {
		return
	}
	group.GET("/custom/turn-logs", handler.List)
	group.GET("/custom/turn-logs/:id", handler.Get)
	group.GET("/custom/turn-log-config", handler.GetConfig)
	group.PUT("/custom/turn-log-config", handler.UpdateConfig)
}
