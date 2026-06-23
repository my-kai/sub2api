package oauthapp

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Bundle 保存自定义 OAuth 应用路由注册所需对象。
type Bundle struct {
	Handler *Handler
	Service *Service
}

// ProvideBundle 初始化自定义 OAuth 应用存储和处理器。
func ProvideBundle(db *sql.DB, userService *service.UserService, authService *service.AuthService) (*Bundle, error) {
	store, err := NewStore(db, defaultCodeTTL)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := store.EnsureSchema(ctx); err != nil {
		return nil, fmt.Errorf("initialize oauth application schema: %w", err)
	}
	svc := NewService(store, userService, authService)
	return &Bundle{Handler: NewHandler(svc), Service: svc}, nil
}
