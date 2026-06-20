package server

import (
	"context"
	"log"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	customactivityhandler "github.com/Wei-Shaw/sub2api/internal/custom/activity/handler"
	customactivityroutes "github.com/Wei-Shaw/sub2api/internal/custom/activity/routes"
	customactivityruntime "github.com/Wei-Shaw/sub2api/internal/custom/activity/runtime"
	customannouncements "github.com/Wei-Shaw/sub2api/internal/custom/announcements/handler"
	customannouncementroutes "github.com/Wei-Shaw/sub2api/internal/custom/announcements/routes"
	customcallbackauth "github.com/Wei-Shaw/sub2api/internal/custom/callbackauth"
	customcallbackauthroutes "github.com/Wei-Shaw/sub2api/internal/custom/callbackauth/routes"
	customimagegen "github.com/Wei-Shaw/sub2api/internal/custom/imagegen"
	customimagegenroutes "github.com/Wei-Shaw/sub2api/internal/custom/imagegen/routes"
	customimagegenhandoff "github.com/Wei-Shaw/sub2api/internal/custom/imagegenhandoff"
	customimagegenhandoffroutes "github.com/Wei-Shaw/sub2api/internal/custom/imagegenhandoff/routes"
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
	userService *service.UserService,
	cfg *config.Config,
	redisClient *redis.Client,
	customActivity *customactivityruntime.Bundle,
	customImageGen *customimagegen.Bundle,
	customCallbackAuth *customcallbackauth.Bundle,
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
	registerRoutes(r, handlers, jwtAuth, adminAuth, apiKeyAuth, apiKeyService, subscriptionService, opsService, settingService, userService, cfg, redisClient, customActivity, customImageGen, customCallbackAuth)

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
	userService *service.UserService,
	cfg *config.Config,
	redisClient *redis.Client,
	customActivity *customactivityruntime.Bundle,
	customImageGen *customimagegen.Bundle,
	customCallbackAuth *customcallbackauth.Bundle,
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
	registerCustomAnnouncementRoutes(v1, cfg.Pricing.DataDir, adminAuth, settingService)
	registerCustomCallbackAuthRoutes(v1, customCallbackAuth, jwtAuth, settingService)
	registerCustomActivityRoutes(v1, customActivity, jwtAuth, adminAuth, settingService)
	registerCustomImageGenHandoffRoutes(v1, cfg.ImageGen, userService, jwtAuth, settingService)
	registerCustomImageGenerationRoutes(v1, customImageGen, jwtAuth, adminAuth, settingService)

	handler.RegisterPageRoutes(v1, cfg.Pricing.DataDir, gin.HandlerFunc(jwtAuth), gin.HandlerFunc(adminAuth), settingService)
}

// registerCustomActivityRoutes 只在主仓路由层追加活动中心入口。
//
// 活动状态、红包雨结算和余额入账都在 internal/custom/activity 内部实现；这里仅复用
// 主仓现有用户/管理员鉴权，减少后续合并上游时的冲突面。
func registerCustomActivityRoutes(
	v1 *gin.RouterGroup,
	bundle *customactivityruntime.Bundle,
	jwtAuth middleware2.JWTAuthMiddleware,
	adminAuth middleware2.AdminAuthMiddleware,
	settingService *service.SettingService,
) {
	if v1 == nil || bundle == nil || bundle.Service == nil {
		return
	}

	h := customactivityhandler.NewHandler(bundle.Service)
	user := v1.Group("")
	user.Use(gin.HandlerFunc(jwtAuth))
	user.Use(middleware2.BackendModeUserGuard(settingService))
	customactivityroutes.RegisterUserRoutes(user, h)

	// WebSocket 领取使用已登录接口签发的一次性 ticket 鉴权；浏览器 WS 不能稳定携带 Bearer header。
	customactivityroutes.RegisterWebSocketRoutes(v1, h)

	admin := v1.Group("/admin")
	admin.Use(gin.HandlerFunc(adminAuth))
	admin.Use(middleware2.AdminComplianceGuard(settingService))
	customactivityroutes.RegisterAdminRoutes(admin, h)
}

// registerCustomAnnouncementRoutes 只在主仓路由层追加 custom 公告图片入口。
//
// 上传、文件名校验和存储策略都在 internal/custom/announcements 内部实现；这里仅复用
// 管理员鉴权与合规确认中间件，避免把二开上传逻辑混进主仓公告 CRUD。
func registerCustomAnnouncementRoutes(
	v1 *gin.RouterGroup,
	dataDir string,
	adminAuth middleware2.AdminAuthMiddleware,
	settingService *service.SettingService,
) {
	if v1 == nil {
		return
	}

	uploadHandler := customannouncements.NewUploadHandler(dataDir)
	customannouncementroutes.RegisterPublicRoutes(v1, uploadHandler)

	admin := v1.Group("/admin")
	admin.Use(gin.HandlerFunc(adminAuth))
	admin.Use(middleware2.AdminComplianceGuard(settingService))
	customannouncementroutes.RegisterAdminRoutes(admin, uploadHandler)
}

// registerCustomCallbackAuthRoutes adds the generic callback login handoff flow.
//
// Consent and code persistence stay in internal/custom/callbackauth; this layer
// only attaches main JWT auth for the browser authorization page. Exchange is
// public because the short-lived one-time code is the bearer credential.
func registerCustomCallbackAuthRoutes(
	v1 *gin.RouterGroup,
	bundle *customcallbackauth.Bundle,
	jwtAuth middleware2.JWTAuthMiddleware,
	settingService *service.SettingService,
) {
	if v1 == nil || bundle == nil || bundle.Handler == nil {
		return
	}

	user := v1.Group("")
	user.Use(gin.HandlerFunc(jwtAuth))
	user.Use(middleware2.BackendModeUserGuard(settingService))
	customcallbackauthroutes.RegisterUserRoutes(user, bundle.Handler)

	customcallbackauthroutes.RegisterExchangeRoutes(v1, bundle.Handler)
}

// registerCustomImageGenHandoffRoutes 只注册 sub2api-ex 到独立 image-gen 的登录态交接入口。
//
// 一次性 code 的生成/消费逻辑放在 internal/custom/imagegenhandoff；这里仅复用
// 主仓用户鉴权与后端模式守卫。exchange 端点不挂用户 JWT，只校验服务间 secret。
func registerCustomImageGenHandoffRoutes(
	v1 *gin.RouterGroup,
	cfg config.ImageGenConfig,
	userService *service.UserService,
	jwtAuth middleware2.JWTAuthMiddleware,
	settingService *service.SettingService,
) {
	if v1 == nil {
		return
	}

	handoffCfg := customimagegenhandoff.Config{
		BaseURL:        cfg.BaseURL,
		ExchangeSecret: cfg.ExchangeSecret,
		CodeTTLSeconds: cfg.CodeTTLSeconds,
	}
	store := customimagegenhandoff.NewMemoryStoreForConfig(handoffCfg)
	handler := customimagegenhandoff.NewHandler(handoffCfg, store, userService)

	user := v1.Group("")
	user.Use(gin.HandlerFunc(jwtAuth))
	user.Use(middleware2.BackendModeUserGuard(settingService))
	customimagegenhandoffroutes.RegisterUserRoutes(user, handler)

	customimagegenhandoffroutes.RegisterServiceRoutes(v1, handler)
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
