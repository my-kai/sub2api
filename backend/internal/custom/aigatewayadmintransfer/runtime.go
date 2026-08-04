package aigatewayadmintransfer

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	migrations "github.com/Wei-Shaw/sub2api/migrations/custom/aigatewayadmintransfer"
)

const (
	// migrationsTableName 将直连来源扣款迁移与主仓及旧 HMAC custom 迁移完全隔离。
	migrationsTableName = "custom_ai_gateway_admin_transfer_schema_migrations"
	// migrationAdvisoryLockID 阻止多实例在启动时并发应用同一批来源扣款迁移。
	migrationAdvisoryLockID int64 = 2026080302
	// migrationTimeout 限制 custom migration 获取连接和锁的最长时间。
	migrationTimeout = 30 * time.Second
)

// Bundle 汇总路由薄接入所需的来源扣款领域对象。
type Bundle struct {
	Store   *Store
	Service *Service
	Handler *Handler
}

// ProvideBundle 执行独立 custom migration 并构造来源直连转入领域对象。
func ProvideBundle(db *sql.DB, userRepo service.UserRepository, totpService *service.TotpService) (*Bundle, error) {
	if db == nil || userRepo == nil || totpService == nil {
		return nil, errors.New("ai-gateway admin transfer dependencies are required")
	}
	migrationCtx, cancel := context.WithTimeout(context.Background(), migrationTimeout)
	defer cancel()
	if err := applyMigrations(migrationCtx, db, migrations.FS); err != nil {
		return nil, err
	}
	store, err := NewStore(db)
	if err != nil {
		return nil, err
	}
	transferService, err := NewService(store, userRepo, totpService)
	if err != nil {
		return nil, err
	}
	return &Bundle{
		Store:   store,
		Service: transferService,
		Handler: NewHandler(transferService),
	}, nil
}

// applyMigrations 使用独立 checksum 表记录 custom SQL，阻止已应用迁移被静默篡改。
func applyMigrations(ctx context.Context, db *sql.DB, source fs.FS) (err error) {
	connection, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("open ai-gateway admin transfer migration connection: %w", err)
	}
	defer connection.Close()
	if _, err := connection.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, migrationAdvisoryLockID); err != nil {
		return fmt.Errorf("lock ai-gateway admin transfer migrations: %w", err)
	}
	defer func() {
		if _, unlockErr := connection.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, migrationAdvisoryLockID); unlockErr != nil && err == nil {
			err = fmt.Errorf("unlock ai-gateway admin transfer migrations: %w", unlockErr)
		}
	}()
	if _, err := connection.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS custom_ai_gateway_admin_transfer_schema_migrations (
			filename TEXT PRIMARY KEY,
			checksum TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("create ai-gateway admin transfer migration table: %w", err)
	}
	files, err := fs.Glob(source, "*.sql")
	if err != nil {
		return fmt.Errorf("list ai-gateway admin transfer migrations: %w", err)
	}
	sort.Strings(files)
	for _, name := range files {
		content, readErr := fs.ReadFile(source, name)
		if readErr != nil {
			return fmt.Errorf("read ai-gateway admin transfer migration %s: %w", name, readErr)
		}
		sqlText := strings.TrimSpace(string(content))
		if sqlText == "" {
			continue
		}
		sum := sha256.Sum256([]byte(sqlText))
		checksum := hex.EncodeToString(sum[:])
		var existingChecksum string
		queryErr := connection.QueryRowContext(ctx, "SELECT checksum FROM "+migrationsTableName+" WHERE filename = $1", name).Scan(&existingChecksum)
		if queryErr == nil {
			if existingChecksum != checksum {
				return fmt.Errorf("ai-gateway admin transfer migration %s checksum mismatch", name)
			}
			continue
		}
		if !errors.Is(queryErr, sql.ErrNoRows) {
			return fmt.Errorf("check ai-gateway admin transfer migration %s: %w", name, queryErr)
		}
		transaction, beginErr := connection.BeginTx(ctx, nil)
		if beginErr != nil {
			return fmt.Errorf("begin ai-gateway admin transfer migration %s: %w", name, beginErr)
		}
		if _, execErr := transaction.ExecContext(ctx, sqlText); execErr != nil {
			_ = transaction.Rollback()
			return fmt.Errorf("apply ai-gateway admin transfer migration %s: %w", name, execErr)
		}
		if _, insertErr := transaction.ExecContext(ctx, "INSERT INTO "+migrationsTableName+" (filename, checksum) VALUES ($1, $2)", name, checksum); insertErr != nil {
			_ = transaction.Rollback()
			return fmt.Errorf("record ai-gateway admin transfer migration %s: %w", name, insertErr)
		}
		if commitErr := transaction.Commit(); commitErr != nil {
			return fmt.Errorf("commit ai-gateway admin transfer migration %s: %w", name, commitErr)
		}
	}
	return nil
}
