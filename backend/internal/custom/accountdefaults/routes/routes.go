package routes

import (
	accountdefaults "github.com/Wei-Shaw/sub2api/internal/custom/accountdefaults"
	"github.com/gin-gonic/gin"
)

// RegisterAdminRoutes registers the system-administrator account-defaults APIs.
func RegisterAdminRoutes(group gin.IRouter, handler *accountdefaults.Handler) {
	if group == nil || handler == nil {
		return
	}
	group.GET("/custom/account-defaults", handler.Get)
	group.PUT("/custom/account-defaults", handler.Update)
	group.GET("/custom/account-defaults/resolve", handler.Resolve)
}
