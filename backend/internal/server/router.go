package server

import (
	"context"
	"log"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	customimagegen "github.com/Wei-Shaw/sub2api/internal/custom/imagegen"
	customimagegenroutes "github.com/Wei-Shaw/sub2api/internal/custom/imagegen/routes"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/server/routes"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/web"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

const frameSrcRefreshTimeout = 5 * time.Second

// SetupRouter 配置路由器中间件和路由
func SetupRouter(
	r *gin.Engine,
	handlers *handler.Handlers,
	jwtAuth middleware2.JWTAuthMiddleware,
	adminAuth middleware2.AdminAuthMiddleware,
	apiKeyAuth middleware2.APIKeyAuthMiddleware,
	apiKeyService *service.APIKeyService,
	subscriptionService *service.SubscriptionService,
	opsService *service.OpsService,
	settingService *service.SettingService,
	cfg *config.Config,
	redisClient *redis.Client,
	customImageGen *customimagegen.Bundle,
) *gin.Engine {
	// 缓存 iframe 页面的 origin 列表，用于动态注入 CSP frame-src
	var cachedFrameOrigins atomic.Pointer[[]string]
	emptyOrigins := []string{}
	cachedFrameOrigins.Store(&emptyOrigins)

	refreshFrameOrigins := func() {
		ctx, cancel := context.WithTimeout(context.Background(), frameSrcRefreshTimeout)
		defer cancel()
		origins, err := settingService.GetFrameSrcOrigins(ctx)
		if err != nil {
			// 获取失败时保留已有缓存，避免 frame-src 被意外清空
			return
		}
		cachedFrameOrigins.Store(&origins)
	}
	refreshFrameOrigins() // 启动时初始化

	// 应用中间件
	r.Use(middleware2.RequestLogger())
	r.Use(middleware2.Logger())
	r.Use(middleware2.CORS(cfg.CORS))
	r.Use(middleware2.SecurityHeaders(cfg.Security.CSP, func() []string {
		if p := cachedFrameOrigins.Load(); p != nil {
			return *p
		}
		return nil
	}))

	// Serve embedded frontend with settings injection if available
	if web.HasEmbeddedFrontend() {
		frontendServer, err := web.NewFrontendServer(settingService)
		if err != nil {
			log.Printf("Warning: Failed to create frontend server with settings injection: %v, using legacy mode", err)
			r.Use(web.ServeEmbeddedFrontend())
			settingService.SetOnUpdateCallback(refreshFrameOrigins)
		} else {
			// Register combined callback: invalidate HTML cache + refresh frame origins
			settingService.SetOnUpdateCallback(func() {
				frontendServer.InvalidateCache()
				refreshFrameOrigins()
			})
			r.Use(frontendServer.Middleware())
		}
	} else {
		settingService.SetOnUpdateCallback(refreshFrameOrigins)
	}

	// 注册路由
	registerRoutes(r, handlers, jwtAuth, adminAuth, apiKeyAuth, apiKeyService, subscriptionService, opsService, settingService, cfg, redisClient, customImageGen)

	return r
}

// registerRoutes 注册所有 HTTP 路由
func registerRoutes(
	r *gin.Engine,
	h *handler.Handlers,
	jwtAuth middleware2.JWTAuthMiddleware,
	adminAuth middleware2.AdminAuthMiddleware,
	apiKeyAuth middleware2.APIKeyAuthMiddleware,
	apiKeyService *service.APIKeyService,
	subscriptionService *service.SubscriptionService,
	opsService *service.OpsService,
	settingService *service.SettingService,
	cfg *config.Config,
	redisClient *redis.Client,
	customImageGen *customimagegen.Bundle,
) {
	// 通用路由（健康检查、状态等）
	routes.RegisterCommonRoutes(r)

	// API v1
	v1 := r.Group("/api/v1")

	// 注册各模块路由
	routes.RegisterAuthRoutes(v1, h, jwtAuth, redisClient, settingService)
	routes.RegisterUserRoutes(v1, h, jwtAuth, settingService)
	routes.RegisterAdminRoutes(v1, h, adminAuth, settingService)
	routes.RegisterGatewayRoutes(r, h, apiKeyAuth, apiKeyService, subscriptionService, opsService, settingService, cfg)
	routes.RegisterPaymentRoutes(v1, h.Payment, h.PaymentWebhook, h.Admin.Payment, jwtAuth, adminAuth, settingService)
	registerCustomImageGenerationRoutes(v1, customImageGen, jwtAuth, adminAuth, settingService)

	handler.RegisterPageRoutes(v1, cfg.Pricing.DataDir, gin.HandlerFunc(jwtAuth), gin.HandlerFunc(adminAuth), settingService)
}

// registerCustomImageGenerationRoutes 只在主仓路由层追加 custom 生图入口。
//
// 生图业务 handler、service、store 都在 internal/custom/imagegen 内部装配；这里仅复用
// 主仓现有鉴权与模式守卫，避免把二开逻辑混入主仓核心路由实现。
func registerCustomImageGenerationRoutes(
	v1 *gin.RouterGroup,
	bundle *customimagegen.Bundle,
	jwtAuth middleware2.JWTAuthMiddleware,
	adminAuth middleware2.AdminAuthMiddleware,
	settingService *service.SettingService,
) {
	if v1 == nil || bundle == nil || bundle.Handler == nil {
		return
	}

	user := v1.Group("")
	user.Use(customimagegenroutes.BearerTokenQueryFallback())
	user.Use(gin.HandlerFunc(jwtAuth))
	user.Use(middleware2.BackendModeUserGuard(settingService))
	customimagegenroutes.RegisterUserRoutes(user, bundle.Handler)

	// 公共图库保持匿名可读；handler 内部会按可用登录态决定是否展示提示词。
	customimagegenroutes.RegisterPublicGalleryRoute(v1, bundle.Handler)

	admin := v1.Group("/admin")
	admin.Use(gin.HandlerFunc(adminAuth))
	admin.Use(middleware2.AdminComplianceGuard(settingService))
	customimagegenroutes.RegisterAdminRoutes(admin, bundle.Handler)
}
