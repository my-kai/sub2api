package routes

import (
	announcementhandler "github.com/Wei-Shaw/sub2api/internal/custom/announcements/handler"
	"github.com/gin-gonic/gin"
)

// RegisterPublicRoutes exposes immutable announcement image files for rendered Markdown.
func RegisterPublicRoutes(group gin.IRouter, h *announcementhandler.UploadHandler) {
	if group == nil || h == nil {
		return
	}
	group.GET("/custom/announcements/images/:filename", h.ServeImage)
}

// RegisterAdminRoutes exposes admin-only upload endpoints used by the Markdown editor.
func RegisterAdminRoutes(group gin.IRouter, h *announcementhandler.UploadHandler) {
	if group == nil || h == nil {
		return
	}
	group.POST("/custom/announcements/images", h.UploadImage)
}
