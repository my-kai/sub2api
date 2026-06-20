package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/custom/callbackauth"
	"github.com/gin-gonic/gin"
)

// RegisterUserRoutes adds authenticated callback authorization endpoints.
func RegisterUserRoutes(group *gin.RouterGroup, handler *callbackauth.Handler) {
	if group == nil || handler == nil {
		return
	}
	group.GET("/custom/callback-auth/authorize", handler.Info)
	group.POST("/custom/callback-auth/authorize", handler.Authorize)
}

// RegisterExchangeRoutes adds the public one-time code exchange endpoint.
func RegisterExchangeRoutes(group *gin.RouterGroup, handler *callbackauth.Handler) {
	if group == nil || handler == nil {
		return
	}
	group.POST("/custom/callback-auth/exchange", handler.Exchange)
}
