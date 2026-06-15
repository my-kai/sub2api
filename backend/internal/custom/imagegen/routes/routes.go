package routes

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/handler"
	"github.com/gin-gonic/gin"
)

// BearerTokenQueryFallback 仅为 EventSource 这类无法设置 Authorization header 的请求兜底。
//
// 前端 SSE 会把当前访问令牌放在 token 查询参数里；这里在进入主仓 JWT 中间件前
// 转成标准 Bearer header，避免修改主仓鉴权实现。
func BearerTokenQueryFallback() gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.TrimSpace(c.GetHeader("Authorization")) == "" {
			if token := strings.TrimSpace(c.Query("token")); token != "" {
				c.Request.Header.Set("Authorization", "Bearer "+token)
			}
		}
		c.Next()
	}
}

// RegisterUserRoutes 注册 custom 生图用户侧接口。
//
// 调用方应在 group 外层套主仓 JWT 鉴权中间件；公共图库如果要匿名可读，
// 可以单独调用 RegisterPublicGalleryRoute 到未鉴权分组。
func RegisterUserRoutes(group gin.IRouter, h *handler.ImageGenerationHandler) {
	if group == nil || h == nil {
		return
	}
	group.GET("/custom/images/status", h.PublicStatus)
	group.GET("/custom/images/models", h.Models)
	group.GET("/custom/images/price-quote", h.PriceQuote)
	group.GET("/custom/images/keys", h.APIKeys)
	group.POST("/custom/images/keys", h.CreateAPIKey)
	group.DELETE("/custom/images/keys/:id", h.DeleteAPIKey)
	group.POST("/custom/images/sessions", h.CreateSession)
	group.GET("/custom/images/sessions", h.Sessions)
	group.PATCH("/custom/images/sessions/:id", h.UpdateSession)
	group.DELETE("/custom/images/sessions/:id", h.DeleteSession)
	group.POST("/custom/images/sessions/:id/current-image", h.SetCurrentImage)
	group.DELETE("/custom/images/sessions/:id/current-image", h.ResetCurrentImage)
	group.GET("/custom/images/sessions/:id/tasks", h.SessionTasks)
	group.GET("/custom/images/sessions/:id/tasks/events", h.SessionTaskEvents)
	group.GET("/custom/images/my-images", h.MyImages)
	group.POST("/custom/images/my-images/:task_id/:image_index/publish", h.PublishMyImage)
	group.POST("/custom/images/my-images/:task_id/:image_index/hide", h.HideMyImage)
	group.POST("/custom/images/tasks", h.CreateTask)
	group.GET("/custom/images/tasks/:id", h.Task)
	group.POST("/custom/images/tasks/:id/retry", h.RetryTask)
	group.POST("/custom/images/tasks/:id/cancel", h.CancelTask)
}

// RegisterPublicGalleryRoute 注册不依赖登录态的 custom 生图接口。
//
// 该函数由主仓未鉴权 v1 分组调用；OpenAI 兼容接口必须放在这里，
// 避免经过 JWT 中间件后把 image key 调用误判成登录 token 失效。
func RegisterPublicGalleryRoute(group gin.IRouter, h *handler.ImageGenerationHandler) {
	if group == nil || h == nil {
		return
	}
	group.GET("/custom/gallery/images", h.PublicGallery)
	group.POST("/custom/openai/v1/images/generations", h.OpenAIImageGenerations)
	group.POST("/custom/openai/v1/images/edits", h.OpenAIImageEdits)
}

// RegisterAdminRoutes 注册 custom 生图管理员接口。
//
// 调用方应在 group 外层套主仓管理员鉴权中间件。
func RegisterAdminRoutes(group gin.IRouter, h *handler.ImageGenerationHandler) {
	if group == nil || h == nil {
		return
	}
	group.GET("/custom/images/config", h.AdminConfig)
	group.PUT("/custom/images/config", h.UpdateAdminConfig)
	group.GET("/custom/images/users/search", h.SearchAdminUsers)
	group.GET("/custom/images/user-limits", h.UserLimits)
	group.PUT("/custom/images/user-limits/:user_id", h.UpsertUserLimit)
	group.DELETE("/custom/images/user-limits/:user_id", h.DeleteUserLimit)
}
