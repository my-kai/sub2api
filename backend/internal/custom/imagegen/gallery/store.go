package gallery

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Store 持有公共图库 SQL 持久化方法。
type Store struct {
	db          *sql.DB
	tablePrefix string
}

// NewStore 基于已迁移的 SQL 连接创建公共图库 store。
func NewStore(db *sql.DB, tablePrefix string) (*Store, error) {
	if db == nil {
		return nil, fmt.Errorf("sql db is required")
	}
	if err := validateTablePrefix(tablePrefix); err != nil {
		return nil, err
	}
	return &Store{db: db, tablePrefix: tablePrefix}, nil
}

// UpsertVisible 新增或恢复显示公共图库记录。
func (s *Store) UpsertVisible(ctx context.Context, input UpsertInput, now time.Time) (Item, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO `+s.table("image_public_gallery_items")+` (
			user_id, source_task_id, source_image_index, image_url, prompt,
			is_visible, created_from_public_generation, published_at, hidden_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, TRUE, $6, $7, NULL, $8, $8)
		ON CONFLICT (source_task_id, source_image_index) DO UPDATE SET
			user_id = EXCLUDED.user_id,
			image_url = EXCLUDED.image_url,
			prompt = EXCLUDED.prompt,
			is_visible = TRUE,
			created_from_public_generation = `+s.table("image_public_gallery_items")+`.created_from_public_generation OR EXCLUDED.created_from_public_generation,
			published_at = EXCLUDED.published_at,
			hidden_at = NULL,
			updated_at = EXCLUDED.updated_at
	`, input.UserID, input.SourceTaskID, input.SourceImageIndex, strings.TrimSpace(input.ImageURL),
		strings.TrimSpace(input.Prompt), input.CreatedFromPublicGeneration, input.PublishedAt.UTC(), now.UTC())
	if err != nil {
		return Item{}, fmt.Errorf("upsert public gallery item: %w", err)
	}
	return s.BySource(ctx, input.SourceTaskID, input.SourceImageIndex)
}

// HideBySource 将指定来源图片隐藏。
func (s *Store) HideBySource(ctx context.Context, sourceTaskID int64, sourceImageIndex int, now time.Time) (Item, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE `+s.table("image_public_gallery_items")+`
		SET is_visible = FALSE, hidden_at = $1, updated_at = $1
		WHERE source_task_id = $2 AND source_image_index = $3
	`, now.UTC(), sourceTaskID, sourceImageIndex)
	if err != nil {
		return Item{}, fmt.Errorf("hide public gallery item: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return Item{}, ErrNotFound
	}
	return s.BySource(ctx, sourceTaskID, sourceImageIndex)
}

// BySource 按任务图片来源读取图库记录。
func (s *Store) BySource(ctx context.Context, sourceTaskID int64, sourceImageIndex int) (Item, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, source_task_id, source_image_index, image_url, prompt,
		       is_visible, created_from_public_generation, published_at, hidden_at, created_at, updated_at
		FROM `+s.table("image_public_gallery_items")+`
		WHERE source_task_id = $1 AND source_image_index = $2
	`, sourceTaskID, sourceImageIndex)
	item, err := scanItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, fmt.Errorf("query public gallery item by source: %w", err)
	}
	return item, nil
}

// ListVisible 返回公共图库可见记录分页。
func (s *Store) ListVisible(ctx context.Context, page int, pageSize int) ([]Item, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 20 {
		pageSize = 20
	}
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+s.table("image_public_gallery_items")+` WHERE is_visible = TRUE`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count public gallery items: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, source_task_id, source_image_index, image_url, prompt,
		       is_visible, created_from_public_generation, published_at, hidden_at, created_at, updated_at
		FROM `+s.table("image_public_gallery_items")+`
		WHERE is_visible = TRUE
		ORDER BY published_at DESC, id DESC
		LIMIT $1 OFFSET $2
	`, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("query public gallery items: %w", err)
	}
	defer rows.Close()
	items := []Item{}
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (s *Store) table(name string) string {
	return s.tablePrefix + name
}

func validateTablePrefix(prefix string) error {
	for _, r := range strings.TrimSpace(prefix) {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return fmt.Errorf("public gallery table prefix is invalid")
	}
	return nil
}

func scanItem(row interface{ Scan(dest ...any) error }) (Item, error) {
	var item Item
	var hiddenAt sql.NullTime
	err := row.Scan(
		&item.ID,
		&item.UserID,
		&item.SourceTaskID,
		&item.SourceImageIndex,
		&item.ImageURL,
		&item.Prompt,
		&item.IsVisible,
		&item.CreatedFromPublicGeneration,
		&item.PublishedAt,
		&hiddenAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return Item{}, err
	}
	item.PublishedAt = item.PublishedAt.UTC()
	item.CreatedAt = item.CreatedAt.UTC()
	item.UpdatedAt = item.UpdatedAt.UTC()
	if hiddenAt.Valid {
		value := hiddenAt.Time.UTC()
		item.HiddenAt = &value
	}
	return item, nil
}
