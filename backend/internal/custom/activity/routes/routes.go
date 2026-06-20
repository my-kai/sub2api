package routes

import (
	activityhandler "github.com/Wei-Shaw/sub2api/internal/custom/activity/handler"
	"github.com/gin-gonic/gin"
)

// RegisterUserRoutes registers logged-in user custom activity routes.
func RegisterUserRoutes(group gin.IRouter, h *activityhandler.Handler) {
	if group == nil || h == nil {
		return
	}
	group.GET("/custom/activities", h.ListUserActivities)
	group.GET("/custom/activities/:id", h.GetUserActivity)
	group.GET("/custom/activities/:id/red-packet-rain/state", h.GetRedPacketRainState)
	group.POST("/custom/activities/:id/red-packet-rain/ws-ticket", h.IssueRedPacketRainWSTicket)
}

// RegisterWebSocketRoutes registers ticket-authenticated user custom activity sockets.
func RegisterWebSocketRoutes(group gin.IRouter, h *activityhandler.Handler) {
	if group == nil || h == nil {
		return
	}
	group.GET("/custom/activities/:id/red-packet-rain/ws", h.ServeRedPacketRainWS)
}

// RegisterAdminRoutes registers custom activity admin routes.
func RegisterAdminRoutes(group gin.IRouter, h *activityhandler.Handler) {
	if group == nil || h == nil {
		return
	}
	group.GET("/custom/activities", h.ListAdminActivities)
	group.POST("/custom/activities", h.CreateAdminActivity)
	group.GET("/custom/activities/:id", h.GetAdminActivity)
	group.PUT("/custom/activities/:id", h.UpdateAdminActivity)
	group.POST("/custom/activities/:id/end", h.EndAdminActivity)
	group.POST("/custom/activities/:id/offline", h.OfflineAdminActivity)
	group.GET("/custom/activities/:id/claims", h.ListAdminClaims)
}
