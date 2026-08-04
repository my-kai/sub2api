// Package routes 注册 ai-gateway 管理员直连来源余额接口。
package routes

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/custom/aigatewayadmintransfer"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

// RequireAdminAPIKey 拒绝未携带 x-api-key 的请求，确保该 custom 接口不接受管理员 JWT 替代服务间凭据。
func RequireAdminAPIKey(c *gin.Context) {
	if strings.TrimSpace(c.GetHeader("x-api-key")) == "" {
		response.ErrorWithDetails(c, http.StatusUnauthorized, "需要管理员 API Key", "AI_GATEWAY_ADMIN_TRANSFER_ADMIN_KEY_REQUIRED", nil)
		c.Abort()
		return
	}
	c.Next()
}

// RegisterAdminRoutes 在已完成管理员 API Key 认证的路由组下注册来源专用接口。
func RegisterAdminRoutes(group gin.IRouter, handler *aigatewayadmintransfer.Handler) {
	if group == nil || handler == nil {
		return
	}
	group.POST("/custom/ai-gateway-transfers/balance", handler.Balance)
	group.POST("/custom/ai-gateway-transfers/debit", handler.Debit)
	group.POST("/custom/ai-gateway-transfers/status", handler.Status)
}
