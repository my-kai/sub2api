package imagegen

import (
	"context"
	"database/sql"
	"log"
	"net/url"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/chatgpt2api"
	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/gallery"
	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/handler"
	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/imagequeue"
	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/runtime"
)

// Options 是 custom 生图模块运行期装配所需的最小配置。
type Options struct {
	TablePrefix        string
	ChatGPT2APIBaseURL *url.URL
	ChatGPT2APIAuthKey string
	HTTPTimeout        time.Duration
	UserResolver       runtime.UserResolver
	AdminUserLookup    runtime.AdminUserLookup
	BalanceCache       imagequeue.BalanceCacheInvalidator
	Logger             *log.Logger
}

// Bundle 保存 custom 生图模块产出的 handler、service、worker。
type Bundle struct {
	ImageClient    *chatgpt2api.Client
	QueueStore     *imagequeue.Store
	QueueService   *imagequeue.Service
	GalleryStore   *gallery.Store
	GalleryService *gallery.Service
	EventHub       *imagequeue.TaskEventHub
	Worker         *imagequeue.Worker
	Handler        *handler.ImageGenerationHandler
}

// NewBundle 装配 custom 生图核心依赖。
//
// 这里只创建对象，不注册主仓路由、不启动 goroutine；tasks-3 的薄接入层负责
// 按主仓生命周期调用 routes.Register* 和 Worker.Run。
func NewBundle(db *sql.DB, opts Options) (*Bundle, error) {
	queueStore, err := imagequeue.NewStore(db, opts.TablePrefix)
	if err != nil {
		return nil, err
	}
	galleryStore, err := gallery.NewStore(db, opts.TablePrefix)
	if err != nil {
		return nil, err
	}
	eventHub := imagequeue.NewTaskEventHub()
	queueService := imagequeue.NewService(queueStore).
		WithTaskEventHub(eventHub).
		WithBalanceCacheInvalidator(opts.BalanceCache)
	galleryService := gallery.NewService(galleryStore)
	timeout := opts.HTTPTimeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	envBaseURL := ""
	if opts.ChatGPT2APIBaseURL != nil {
		envBaseURL = opts.ChatGPT2APIBaseURL.String()
	}
	if err := queueStore.SeedChatGPT2APIConfigFromEnv(context.Background(), envBaseURL, opts.ChatGPT2APIAuthKey); err != nil {
		return nil, err
	}
	imageClient := chatgpt2api.NewClient(opts.ChatGPT2APIBaseURL, opts.ChatGPT2APIAuthKey, timeout).
		WithConfigLoader(queueService.ChatGPT2APIRuntimeConfig)
	worker := imagequeue.NewWorker(queueStore, queueService, imageClient, imagequeue.WorkerOptions{
		Logger:         opts.Logger,
		GalleryService: galleryService,
	})
	httpHandler := handler.NewImageGenerationHandler(opts.UserResolver, queueService).
		WithGalleryService(galleryService).
		WithAdminUserLookup(opts.AdminUserLookup)
	return &Bundle{
		ImageClient:    imageClient,
		QueueStore:     queueStore,
		QueueService:   queueService,
		GalleryStore:   galleryStore,
		GalleryService: galleryService,
		EventHub:       eventHub,
		Worker:         worker,
		Handler:        httpHandler,
	}, nil
}

// RunWorker 启动后台 worker，给主仓启动装配层一个明确入口。
func (b *Bundle) RunWorker(ctx context.Context) {
	if b == nil || b.Worker == nil {
		return
	}
	b.Worker.Run(ctx)
}
