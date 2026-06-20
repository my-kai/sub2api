package callbackauth

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Bundle keeps the callback-auth handler ready for the main router.
type Bundle struct {
	Handler *Handler
}

// ProvideBundle initializes callback-auth storage and handler dependencies.
func ProvideBundle(db *sql.DB, userService *service.UserService, settingService *service.SettingService) (*Bundle, error) {
	store, err := NewStore(db, defaultCodeTTL)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := store.EnsureSchema(ctx); err != nil {
		return nil, fmt.Errorf("initialize callback auth schema: %w", err)
	}
	return &Bundle{
		Handler: NewHandler(store, userService, settingService),
	}, nil
}
