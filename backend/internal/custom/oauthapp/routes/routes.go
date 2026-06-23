package routes

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/custom/oauthapp"
	"github.com/gin-gonic/gin"
)

// RegisterAdminRoutes 在 /admin 下注册应用管理接口。
func RegisterAdminRoutes(group *gin.RouterGroup, handler *oauthapp.Handler) {
	if group == nil || handler == nil {
		return
	}
	apps := group.Group("/custom/oauth-applications")
	{
		apps.GET("", handler.ListApplications)
		apps.POST("", handler.CreateApplication)
		apps.PUT("/:id", handler.UpdateApplication)
		apps.POST("/:id/reset-secret", handler.ResetSecret)
		apps.DELETE("/:id", handler.DeleteApplication)
	}
}

// RegisterUserRoutes 注册需要登录用户访问的授权确认接口。
func RegisterUserRoutes(group *gin.RouterGroup, handler *oauthapp.Handler) {
	if group == nil || handler == nil {
		return
	}
	oauth := group.Group("/custom/oauth")
	{
		oauth.GET("/authorize", handler.AuthorizeInfo)
		oauth.POST("/authorize", handler.AuthorizeConfirm)
	}
}

// UseAuthorizePageRedirect 在认证前插入 authorize 浏览器跳转中间件。
// 它只处理 HTML 导航请求；XHR/API 请求会继续进入受保护的用户路由组。
func UseAuthorizePageRedirect(group *gin.RouterGroup, handler *oauthapp.Handler) {
	if group == nil || handler == nil {
		return
	}
	group.Use(func(c *gin.Context) {
		// 中间件挂在 /api/v1 上；用后缀匹配可以兼容反向代理额外加的路径前缀。
		path := strings.TrimRight(c.Request.URL.Path, "/")
		if c.Request.Method == http.MethodGet && strings.HasSuffix(path, "/custom/oauth/authorize") {
			handler.RedirectAuthorizePageIfHTML(c)
			return
		}
		c.Next()
	})
}

// RegisterPublicRoutes 注册 OAuth token 换取接口。
func RegisterPublicRoutes(group *gin.RouterGroup, handler *oauthapp.Handler) {
	if group == nil || handler == nil {
		return
	}
	group.POST("/custom/oauth/token", handler.Token)
}
