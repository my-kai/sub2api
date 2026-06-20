package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/custom/imagegenhandoff"
	"github.com/gin-gonic/gin"
)

// RegisterUserRoutes adds authenticated browser handoff endpoints.
func RegisterUserRoutes(group *gin.RouterGroup, handler *imagegenhandoff.Handler) {
	if group == nil || handler == nil {
		return
	}
	group.POST("/custom/image-gen/login-code", handler.LoginCode)
}

// RegisterServiceRoutes adds trusted service-to-service exchange endpoints.
func RegisterServiceRoutes(group *gin.RouterGroup, handler *imagegenhandoff.Handler) {
	if group == nil || handler == nil {
		return
	}
	group.POST("/custom/image-gen/exchange", handler.Exchange)
}
