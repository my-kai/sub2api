package imagegen

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/runtime"
	"github.com/Wei-Shaw/sub2api/internal/service"
	imagegenmigrations "github.com/Wei-Shaw/sub2api/migrations/custom/imagegen"
)

// ProvideBundle 完成 custom 生图模块启动期装配。
//
// 这里集中执行 custom SQL、读取 custom 环境变量并装配 worker；主仓启动文件只需要调用
// 这一处，避免把二开业务细节散落到 Wire 生成的大文件里。
func ProvideBundle(db *sql.DB, userService *service.UserService, adminService service.AdminService) (*Bundle, error) {
	cfg, err := runtime.LoadConfigFromEnv()
	if err != nil {
		return nil, fmt.Errorf("load custom imagegen config: %w", err)
	}
	if err := runtime.ValidateTablePrefix(cfg.TablePrefix); err != nil {
		return nil, fmt.Errorf("validate custom imagegen table prefix: %w", err)
	}

	migrationCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTPTimeout)
	defer cancel()
	if err := runtime.ApplyMigrationsFS(migrationCtx, db, imagegenmigrations.FS, runtime.MigrationOptions{
		TablePrefix: cfg.TablePrefix,
	}); err != nil {
		return nil, err
	}

	userReader := runtime.NewMainUserServiceReader(userService)
	return NewBundle(db, Options{
		TablePrefix:        cfg.TablePrefix,
		ChatGPT2APIBaseURL: cfg.ChatGPT2APIBaseURL,
		ChatGPT2APIAuthKey: cfg.ChatGPT2APIAuthKey,
		HTTPTimeout:        cfg.HTTPTimeout,
		UserResolver:       runtime.NewContextUserResolver(userReader),
		AdminUserLookup:    runtime.NewMainAdminUserLookup(adminService),
		Logger:             log.Default(),
	})
}
