package imagequeue

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/chatgpt2api"
)

// Store 持有 custom 生图队列的 SQL 持久化方法。
//
// 表结构由 backend/migrations/custom/imagegen/001_image_generation.sql 维护；
// Store 不做自动建表，避免运行期 schema 漂移。
type Store struct {
	db          *sql.DB
	tablePrefix string
}

type sqlExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

const (
	imageGenerationRedeemType = "image_generation"
	imageGenerationChargeNote = "AI 生图扣费"
	imageGenerationRefundNote = "AI 生图退款"
)

// NewStore 基于已迁移的 SQL 连接创建 store。
func NewStore(db *sql.DB, tablePrefix string) (*Store, error) {
	if db == nil {
		return nil, fmt.Errorf("sql db is required")
	}
	if err := validateTablePrefix(tablePrefix); err != nil {
		return nil, err
	}
	return &Store{db: db, tablePrefix: tablePrefix}, nil
}

// Close 保留旧调用方生命周期接口；Store 不拥有 db，不主动关闭。
func (s *Store) Close() error {
	return nil
}

// DB 暴露底层 SQL 连接，主要用于测试和后续薄接入自检。
func (s *Store) DB() *sql.DB {
	if s == nil {
		return nil
	}
	return s.db
}

// CreateChargedJob 在同一个数据库事务内完成余额扣减、余额历史写入和 queued 任务创建。
//
// 这里不用主仓 userRepo.DeductBalance，因为它允许透支；生图创建必须用 SQL 条件保证
// “余额足够才扣款和入队”，避免用户并发提交时出现超扣。
func (s *Store) CreateChargedJob(ctx context.Context, job Job, amount string, chargedAt time.Time) (Job, error) {
	if chargedAt.IsZero() {
		chargedAt = time.Now().UTC()
	}
	amount = formatDecimalString(amount, 5)
	if isZeroDecimalString(amount) {
		job.ChargeStatus = ChargeStatusNone
		return s.CreateJob(ctx, job)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, fmt.Errorf("begin image generation charge transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := deductUserBalance(ctx, tx, job.UserID, amount); err != nil {
		return Job{}, err
	}

	keySeed := balanceLifecycleKeySeed(job, chargedAt)
	job.BalanceIdempotencyKey = balanceLifecycleKey(keySeed)
	job.ChargeAmount = amount
	job.ChargeStatus = ChargeStatusSuccess
	job.ChargeMessage = ""
	created, err := s.createJobWithExecutor(ctx, tx, job)
	if err != nil {
		return Job{}, err
	}
	if err := insertImageGenerationBalanceHistory(ctx, tx, created.UserID, created.BalanceIdempotencyKey, imageGenerationChargeNote, "-"+amount, chargedAt); err != nil {
		return Job{}, err
	}
	if err := tx.Commit(); err != nil {
		return Job{}, fmt.Errorf("commit image generation charge transaction: %w", err)
	}
	return s.GetJob(ctx, created.ID)
}

// RefundChargedJob 只对 charge_status=success 的任务做一次性退款补偿。
//
// 余额和 redeem_codes 历史记录与任务退款状态必须同事务更新；如果任何一步失败，
// 任务会被标记为 refund_failed，方便后台按 charge_message 人工补偿。
func (s *Store) RefundChargedJob(ctx context.Context, job Job, reason string, refundedAt time.Time) (Job, error) {
	if job.ID <= 0 {
		return Job{}, ErrJobNotFound
	}
	if job.ChargeStatus != ChargeStatusSuccess {
		return job, nil
	}
	amount := formatDecimalString(job.ChargeAmount, 5)
	if isZeroDecimalString(amount) {
		return s.MarkRefundSucceeded(ctx, job.ID)
	}
	if refundedAt.IsZero() {
		refundedAt = time.Now().UTC()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, fmt.Errorf("begin image generation refund transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	locked, err := markRefundPending(ctx, tx, s.table("image_generation_jobs"), job.ID)
	if err != nil {
		return Job{}, err
	}
	if !locked {
		return s.GetJob(ctx, job.ID)
	}
	if err := addUserBalance(ctx, tx, job.UserID, amount); err != nil {
		return Job{}, err
	}
	refundKey := job.BalanceIdempotencyKey
	if strings.TrimSpace(refundKey) == "" {
		refundKey = balanceLifecycleKey(fmt.Sprintf("imagegen:%d:%d", job.UserID, job.ID))
	}
	note := imageGenerationRefundNote
	if strings.TrimSpace(reason) != "" {
		note += "：" + strings.TrimSpace(reason)
	}
	if err := insertImageGenerationBalanceHistory(ctx, tx, job.UserID, balanceLifecycleKey(refundKey+":refund"), note, amount, refundedAt); err != nil {
		return Job{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE `+s.table("image_generation_jobs")+`
		SET charge_status = $1, charge_message = NULL
		WHERE id = $2 AND charge_status = $3
	`, ChargeStatusRefunded, job.ID, ChargeStatusSuccess); err != nil {
		return Job{}, fmt.Errorf("mark image generation refund succeeded: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Job{}, fmt.Errorf("commit image generation refund transaction: %w", err)
	}
	return s.GetJob(ctx, job.ID)
}

// GetConfig 读取唯一生效的生图并发配置。
func (s *Store) GetConfig(ctx context.Context) (Config, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT enabled, platform_concurrency, default_user_concurrency, retention_days,
		       unit_price_low, unit_price_medium, unit_price_high,
		       chatgpt2api_base_url, chatgpt2api_auth_key, updated_by_user_id, updated_at
		FROM `+s.table("image_generation_config")+`
		WHERE id = 1
	`)
	var cfg Config
	var updatedBy sql.NullInt64
	if err := row.Scan(
		&cfg.Enabled,
		&cfg.PlatformConcurrency,
		&cfg.DefaultUserConcurrency,
		&cfg.RetentionDays,
		&cfg.UnitPrices.OneK,
		&cfg.UnitPrices.TwoK,
		&cfg.UnitPrices.FourK,
		&cfg.ChatGPT2API.BaseURL,
		&cfg.ChatGPT2API.AuthKey,
		&updatedBy,
		&cfg.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Config{}, ErrJobNotFound
		}
		return Config{}, fmt.Errorf("query image generation config: %w", err)
	}
	cfg.UpdatedByUserID = nullInt64(updatedBy)
	cfg.UpdatedAt = cfg.UpdatedAt.UTC()
	cfg.ChatGPT2API.AuthKeyConfigured = strings.TrimSpace(cfg.ChatGPT2API.AuthKey) != ""
	return cfg, nil
}

// UpsertConfig 保存管理员控制的全局并发配置。
func (s *Store) UpsertConfig(ctx context.Context, cfg Config) error {
	if cfg.UpdatedAt.IsZero() {
		cfg.UpdatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO `+s.table("image_generation_config")+` (
			id, enabled, platform_concurrency, default_user_concurrency, retention_days,
			unit_price_auto, unit_price_low, unit_price_medium, unit_price_high,
			chatgpt2api_base_url, chatgpt2api_auth_key, chatgpt2api_env_seeded,
			updated_by_user_id, updated_at
		) VALUES (1, $1, $2, $3, $4, $5, $5, $6, $7, $8, $9, TRUE, $10, $11)
		ON CONFLICT (id) DO UPDATE SET
			enabled = EXCLUDED.enabled,
			platform_concurrency = EXCLUDED.platform_concurrency,
			default_user_concurrency = EXCLUDED.default_user_concurrency,
			retention_days = EXCLUDED.retention_days,
			unit_price_auto = EXCLUDED.unit_price_auto,
			unit_price_low = EXCLUDED.unit_price_low,
			unit_price_medium = EXCLUDED.unit_price_medium,
			unit_price_high = EXCLUDED.unit_price_high,
			chatgpt2api_base_url = EXCLUDED.chatgpt2api_base_url,
			chatgpt2api_auth_key = EXCLUDED.chatgpt2api_auth_key,
			chatgpt2api_env_seeded = TRUE,
			updated_by_user_id = EXCLUDED.updated_by_user_id,
			updated_at = EXCLUDED.updated_at
	`, cfg.Enabled, cfg.PlatformConcurrency, cfg.DefaultUserConcurrency, cfg.RetentionDays,
		cfg.UnitPrices.OneK, cfg.UnitPrices.TwoK, cfg.UnitPrices.FourK,
		cfg.ChatGPT2API.BaseURL, cfg.ChatGPT2API.AuthKey,
		nullInt64Param(cfg.UpdatedByUserID), cfg.UpdatedAt.UTC())
	if err != nil {
		return fmt.Errorf("upsert image generation config: %w", err)
	}
	return nil
}

// SeedChatGPT2APIConfigFromEnv 把旧环境变量配置迁入 DB，且只在管理员尚未保存过页面配置时执行。
func (s *Store) SeedChatGPT2APIConfigFromEnv(ctx context.Context, baseURL string, authKey string) error {
	baseURL = strings.TrimSpace(baseURL)
	authKey = strings.TrimSpace(authKey)
	if baseURL == "" && authKey == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE `+s.table("image_generation_config")+`
		SET chatgpt2api_base_url = CASE WHEN chatgpt2api_base_url = '' THEN $1 ELSE chatgpt2api_base_url END,
		    chatgpt2api_auth_key = CASE WHEN chatgpt2api_auth_key = '' THEN $2 ELSE chatgpt2api_auth_key END,
		    chatgpt2api_env_seeded = TRUE
		WHERE id = 1
		  AND chatgpt2api_env_seeded = FALSE
		  AND (chatgpt2api_base_url = '' OR chatgpt2api_auth_key = '')
	`, baseURL, authKey)
	if err != nil {
		return fmt.Errorf("seed image generation chatgpt2api config: %w", err)
	}
	return nil
}

// ListUserLimits 返回所有用户并发覆盖，空数据时固定返回空数组。
func (s *Store) ListUserLimits(ctx context.Context) ([]UserLimit, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT user_id, username, email, concurrency, updated_at
		FROM `+s.table("image_generation_user_limits")+`
		ORDER BY user_id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query image generation user limits: %w", err)
	}
	defer rows.Close()
	items := []UserLimit{}
	for rows.Next() {
		var item UserLimit
		var username, email sql.NullString
		if err := rows.Scan(&item.UserID, &username, &email, &item.Concurrency, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan image generation user limit: %w", err)
		}
		item.Username = nullString(username)
		item.Email = nullString(email)
		item.UpdatedAt = item.UpdatedAt.UTC()
		items = append(items, item)
	}
	return items, rows.Err()
}

// UpsertUserLimit 保存单个用户的并发覆盖。
func (s *Store) UpsertUserLimit(ctx context.Context, limit UserLimit) (UserLimit, error) {
	if limit.UpdatedAt.IsZero() {
		limit.UpdatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO `+s.table("image_generation_user_limits")+` (user_id, username, email, concurrency, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id) DO UPDATE SET
			username = EXCLUDED.username,
			email = EXCLUDED.email,
			concurrency = EXCLUDED.concurrency,
			updated_at = EXCLUDED.updated_at
	`, limit.UserID, nullStringParam(limit.Username), nullStringParam(limit.Email), limit.Concurrency, limit.UpdatedAt.UTC())
	if err != nil {
		return UserLimit{}, fmt.Errorf("upsert image generation user limit: %w", err)
	}
	return limit, nil
}

// DeleteUserLimit 删除某个用户的并发覆盖。
func (s *Store) DeleteUserLimit(ctx context.Context, userID int64) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM `+s.table("image_generation_user_limits")+` WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("delete image generation user limit: %w", err)
	}
	return nil
}

// EffectiveUserConcurrency 返回用户级并发覆盖或默认值。
func (s *Store) EffectiveUserConcurrency(ctx context.Context, userID int64, defaultConcurrency int) (int, error) {
	var concurrency int
	err := s.db.QueryRowContext(ctx, `SELECT concurrency FROM `+s.table("image_generation_user_limits")+` WHERE user_id = $1`, userID).Scan(&concurrency)
	if errors.Is(err, sql.ErrNoRows) {
		return defaultConcurrency, nil
	}
	if err != nil {
		return 0, fmt.Errorf("query image generation user concurrency: %w", err)
	}
	return concurrency, nil
}

// CreateSession 保存一个用户生图会话。
func (s *Store) CreateSession(ctx context.Context, session Session) (Session, error) {
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now().UTC()
	}
	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = session.CreatedAt
	}
	row := s.db.QueryRowContext(ctx, `
		INSERT INTO `+s.table("image_generation_sessions")+` (
			user_id, username, email, title, title_customized, current_image_task_id, current_image_index,
			last_task_id, created_at, updated_at, deleted_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NULL)
		RETURNING id
	`, session.UserID, nullStringParam(session.Username), nullStringParam(session.Email), session.Title,
		session.TitleCustomized, nullInt64Param(session.CurrentImageTaskID), nullIntParam(session.CurrentImageIndex),
		nullInt64Param(session.LastTaskID), session.CreatedAt.UTC(), session.UpdatedAt.UTC())
	if err := row.Scan(&session.ID); err != nil {
		return Session{}, fmt.Errorf("create image generation session: %w", err)
	}
	return session, nil
}

// GetSession 按 ID 读取会话，不做用户隔离。
func (s *Store) GetSession(ctx context.Context, id int64) (Session, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+sessionColumns()+` FROM `+s.table("image_generation_sessions")+` WHERE id = $1`, id)
	return scanSession(row)
}

// GetUserSession 按用户 ID 和会话 ID 读取未删除会话。
func (s *Store) GetUserSession(ctx context.Context, userID int64, sessionID int64) (Session, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+sessionColumns()+` FROM `+s.table("image_generation_sessions")+`
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
	`, sessionID, userID)
	return scanSession(row)
}

// ListSessions 返回当前用户最近活跃的 Session。
func (s *Store) ListSessions(ctx context.Context, userID int64, limit int) ([]Session, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+sessionColumns()+` FROM `+s.table("image_generation_sessions")+`
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY updated_at DESC, id DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("query image generation sessions: %w", err)
	}
	defer rows.Close()
	items := []Session{}
	for rows.Next() {
		item, err := scanSessionRows(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// ListSessionTitles 返回用户历史会话标题，用于生成默认标题。
func (s *Store) ListSessionTitles(ctx context.Context, userID int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT title FROM `+s.table("image_generation_sessions")+` WHERE user_id = $1`, userID)
	if err != nil {
		return nil, fmt.Errorf("query image generation session titles: %w", err)
	}
	defer rows.Close()
	var titles []string
	for rows.Next() {
		var title string
		if err := rows.Scan(&title); err != nil {
			return nil, err
		}
		titles = append(titles, title)
	}
	return titles, rows.Err()
}

// UpdateSessionTitle 修改用户自己的 Session 标题。
func (s *Store) UpdateSessionTitle(ctx context.Context, userID int64, sessionID int64, title string, updatedAt time.Time) (Session, error) {
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE `+s.table("image_generation_sessions")+`
		SET title = $1, title_customized = TRUE, updated_at = $2
		WHERE id = $3 AND user_id = $4 AND deleted_at IS NULL
	`, title, updatedAt.UTC(), sessionID, userID)
	if err != nil {
		return Session{}, fmt.Errorf("update image generation session title: %w", err)
	}
	return s.changedUserSession(ctx, result, userID, sessionID)
}

// AutoNameSessionFromPrompt 在会话仍未自定义命名且只包含当前首个任务时，使用提示词摘要命名。
func (s *Store) AutoNameSessionFromPrompt(ctx context.Context, userID int64, sessionID int64, firstTaskID int64, title string, updatedAt time.Time) (bool, error) {
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE `+s.table("image_generation_sessions")+`
		SET title = $1, updated_at = $2
		WHERE id = $3
		  AND user_id = $4
		  AND deleted_at IS NULL
		  AND title_customized = FALSE
		  AND (last_task_id IS NULL OR last_task_id = $5)
	`, title, updatedAt.UTC(), sessionID, userID, firstTaskID)
	if err != nil {
		return false, fmt.Errorf("auto name image generation session: %w", err)
	}
	affected, _ := result.RowsAffected()
	return affected > 0, nil
}

// SoftDeleteSession 软删除当前用户自己的 Session。
func (s *Store) SoftDeleteSession(ctx context.Context, userID int64, sessionID int64, deletedAt time.Time) error {
	if deletedAt.IsZero() {
		deletedAt = time.Now().UTC()
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE `+s.table("image_generation_sessions")+`
		SET deleted_at = $1, updated_at = $1
		WHERE id = $2 AND user_id = $3 AND deleted_at IS NULL
	`, deletedAt.UTC(), sessionID, userID)
	if err != nil {
		return fmt.Errorf("delete image generation session: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrSessionNotFound
	}
	return nil
}

// SetSessionCurrentImage 更新当前编辑来源图。
func (s *Store) SetSessionCurrentImage(ctx context.Context, userID int64, sessionID int64, taskID int64, imageIndex int, updatedAt time.Time) (Session, error) {
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE `+s.table("image_generation_sessions")+`
		SET current_image_task_id = $1, current_image_index = $2, updated_at = $3
		WHERE id = $4 AND user_id = $5 AND deleted_at IS NULL
	`, taskID, imageIndex, updatedAt.UTC(), sessionID, userID)
	if err != nil {
		return Session{}, fmt.Errorf("set image generation session current image: %w", err)
	}
	return s.changedUserSession(ctx, result, userID, sessionID)
}

// ResetSessionCurrentImage 重置当前编辑来源图；若会话内已有完成图片，则恢复到最新一张。
func (s *Store) ResetSessionCurrentImage(ctx context.Context, userID int64, sessionID int64, updatedAt time.Time) (Session, error) {
	if latest, ok, err := s.LatestSessionImageReference(ctx, userID, sessionID); err != nil {
		return Session{}, err
	} else if ok {
		return s.SetSessionCurrentImage(ctx, userID, sessionID, latest.TaskID, latest.ImageIndex, updatedAt)
	}
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE `+s.table("image_generation_sessions")+`
		SET current_image_task_id = NULL, current_image_index = NULL, updated_at = $1
		WHERE id = $2 AND user_id = $3 AND deleted_at IS NULL
	`, updatedAt.UTC(), sessionID, userID)
	if err != nil {
		return Session{}, fmt.Errorf("reset image generation session current image: %w", err)
	}
	return s.changedUserSession(ctx, result, userID, sessionID)
}

// LatestSessionImageReference 返回当前会话最新完成任务中的第一张有效图片。
func (s *Store) LatestSessionImageReference(ctx context.Context, userID int64, sessionID int64) (ImageReference, bool, error) {
	result, err := s.ListSessionJobs(ctx, userID, sessionID, PageRequest{Page: 1, PageSize: 20})
	if err != nil {
		return ImageReference{}, false, err
	}
	for _, job := range result.Items {
		if job.Status != JobStatusCompleted || job.Result == nil {
			continue
		}
		for index, image := range job.Result.Data {
			if strings.TrimSpace(image.URL) != "" {
				return ImageReference{TaskID: job.ID, ImageIndex: index}, true, nil
			}
		}
	}
	return ImageReference{}, false, nil
}

// TouchSessionLastTask 记录会话最后任务并刷新排序时间。
func (s *Store) TouchSessionLastTask(ctx context.Context, sessionID int64, taskID int64, updatedAt time.Time) error {
	return s.touchSessionLastTaskWithExecutor(ctx, s.db, sessionID, taskID, updatedAt)
}

func (s *Store) touchSessionLastTaskWithExecutor(ctx context.Context, exec sqlExecutor, sessionID int64, taskID int64, updatedAt time.Time) error {
	if sessionID <= 0 {
		return nil
	}
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	if _, err := exec.ExecContext(ctx, `
		UPDATE `+s.table("image_generation_sessions")+`
		SET last_task_id = $1, updated_at = $2
		WHERE id = $3
	`, taskID, updatedAt.UTC(), sessionID); err != nil {
		return fmt.Errorf("touch image generation session last task: %w", err)
	}
	return nil
}

// CreateJob 保存 queued 任务。
func (s *Store) CreateJob(ctx context.Context, job Job) (Job, error) {
	created, err := s.createJobWithExecutor(ctx, s.db, job)
	if err != nil {
		return Job{}, err
	}
	return s.GetJob(ctx, created.ID)
}

func (s *Store) createJobWithExecutor(ctx context.Context, exec sqlExecutor, job Job) (Job, error) {
	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now().UTC()
	}
	if job.Status == "" {
		job.Status = JobStatusQueued
	}
	if job.GenerationMode == "" {
		job.GenerationMode = GenerationModeGenerate
	}
	if strings.TrimSpace(job.ChargeAmount) == "" {
		job.ChargeAmount = "0"
	}
	if job.ChargeStatus == "" {
		job.ChargeStatus = ChargeStatusNone
	}
	resultJSON, err := encodeResult(job.Result)
	if err != nil {
		return Job{}, err
	}
	row := exec.QueryRowContext(ctx, `
		INSERT INTO `+s.table("image_generation_jobs")+` (
			user_id, username, email, status, session_id, generation_mode,
			source_image_task_id, source_image_index, source_image_bytes, source_image_filename,
			source_image_content_type, model, prompt, n, quality, size, publish_to_gallery,
			charge_amount, charge_status, balance_idempotency_key, charge_message,
			result_json, error_message, created_at, started_at, finished_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10,
			$11, $12, $13, $14, $15, $16, $17,
			$18, $19, $20, $21,
			$22::jsonb, $23, $24, $25, $26
		)
		RETURNING id
	`, job.UserID, nullStringParam(job.Username), nullStringParam(job.Email), job.Status,
		nullInt64Param(job.SessionID), job.GenerationMode, nullInt64Param(job.SourceImageTaskID),
		nullIntParam(job.SourceImageIndex), nullBytesParam(job.SourceImageBytes),
		nullStringParam(job.SourceImageFilename), nullStringParam(job.SourceImageContentType),
		job.Model, job.Prompt, job.N, nullStringParam(job.Quality), nullStringParam(job.Size),
		job.PublishToGallery, job.ChargeAmount, job.ChargeStatus, nullStringParam(job.BalanceIdempotencyKey),
		nullStringParam(job.ChargeMessage), resultJSON, nullStringParam(job.ErrorMessage),
		job.CreatedAt.UTC(), nullTimeParam(job.StartedAt), nullTimeParam(job.FinishedAt))
	if err := row.Scan(&job.ID); err != nil {
		return Job{}, fmt.Errorf("create image generation job: %w", err)
	}
	if err := s.touchSessionLastTaskWithExecutor(ctx, exec, job.SessionID, job.ID, job.CreatedAt); err != nil {
		return Job{}, err
	}
	return job, nil
}

// GetJob 按 ID 读取任务。
func (s *Store) GetJob(ctx context.Context, id int64) (Job, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+jobColumns()+` FROM `+s.table("image_generation_jobs")+` WHERE id = $1`, id)
	return scanJob(row)
}

// ListSessionJobs 返回会话任务分页。
func (s *Store) ListSessionJobs(ctx context.Context, userID int64, sessionID int64, page PageRequest) (PageResult[Job], error) {
	page = normalizePageRequest(page)
	var total int64
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM `+s.table("image_generation_jobs")+`
		WHERE user_id = $1 AND session_id = $2
	`, userID, sessionID).Scan(&total); err != nil {
		return PageResult[Job]{Page: page.Page, PageSize: page.PageSize, Items: []Job{}}, fmt.Errorf("count image generation session jobs: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+jobColumns()+` FROM `+s.table("image_generation_jobs")+`
		WHERE user_id = $1 AND session_id = $2
		ORDER BY created_at DESC, id DESC
		LIMIT $3 OFFSET $4
	`, userID, sessionID, page.PageSize, (page.Page-1)*page.PageSize)
	if err != nil {
		return PageResult[Job]{Page: page.Page, PageSize: page.PageSize, Items: []Job{}}, fmt.Errorf("query image generation session jobs: %w", err)
	}
	defer rows.Close()
	items, err := scanJobRows(rows)
	if err != nil {
		return PageResult[Job]{Page: page.Page, PageSize: page.PageSize, Items: []Job{}}, err
	}
	return PageResult[Job]{Page: page.Page, PageSize: page.PageSize, Total: total, Pages: pageCount(total, page.PageSize), Items: items}, nil
}

// ListMyImages 返回当前用户已完成任务里的图片分页。
func (s *Store) ListMyImages(ctx context.Context, userID int64, page PageRequest) (PageResult[MyImage], error) {
	page = normalizePageRequest(page)
	jobs, err := s.completedJobsForImages(ctx, userID)
	if err != nil {
		return PageResult[MyImage]{Page: page.Page, PageSize: page.PageSize, Items: []MyImage{}}, err
	}
	all := []MyImage{}
	for _, job := range jobs {
		if job.Result == nil {
			continue
		}
		for index, image := range job.Result.Data {
			if strings.TrimSpace(image.URL) == "" {
				continue
			}
			galleryItemID, inGallery := s.galleryStateForSource(ctx, job.ID, index)
			all = append(all, MyImage{
				TaskID:        job.ID,
				ImageIndex:    index,
				URL:           image.URL,
				CreatedAt:     job.CreatedAt,
				Prompt:        job.Prompt,
				GalleryItemID: galleryItemID,
				InGallery:     inGallery,
			})
		}
	}
	total := int64(len(all))
	start := (page.Page - 1) * page.PageSize
	if start >= len(all) {
		return PageResult[MyImage]{Page: page.Page, PageSize: page.PageSize, Total: total, Pages: pageCount(total, page.PageSize), Items: []MyImage{}}, nil
	}
	end := start + page.PageSize
	if end > len(all) {
		end = len(all)
	}
	return PageResult[MyImage]{Page: page.Page, PageSize: page.PageSize, Total: total, Pages: pageCount(total, page.PageSize), Items: all[start:end]}, nil
}

// QueuePosition 计算 queued 任务当前位置，非 queued 返回 0。
func (s *Store) QueuePosition(ctx context.Context, id int64) (int, error) {
	job, err := s.GetJob(ctx, id)
	if err != nil {
		return 0, err
	}
	if job.Status != JobStatusQueued {
		return 0, nil
	}
	var position int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM `+s.table("image_generation_jobs")+`
		WHERE status = $1 AND (created_at, id) <= ($2, $3)
	`, JobStatusQueued, job.CreatedAt.UTC(), job.ID).Scan(&position); err != nil {
		return 0, fmt.Errorf("query image generation queue position: %w", err)
	}
	return position, nil
}

// UpdateBalanceIdempotencyKey 绑定任务级余额生命周期主键。
func (s *Store) UpdateBalanceIdempotencyKey(ctx context.Context, id int64, key string) (Job, error) {
	return s.updateJobReturning(ctx, id, `balance_idempotency_key = $1`, strings.TrimSpace(key))
}

// MarkChargeSucceeded 标记扣费成功。
func (s *Store) MarkChargeSucceeded(ctx context.Context, id int64) (Job, error) {
	return s.updateJobReturning(ctx, id, `charge_status = $1, charge_message = NULL`, ChargeStatusSuccess)
}

// MarkChargeSkipped 标记任务无需扣费。
func (s *Store) MarkChargeSkipped(ctx context.Context, id int64) (Job, error) {
	return s.updateJobReturning(ctx, id, `charge_status = $1, charge_message = NULL`, ChargeStatusNone)
}

// MarkChargeFailed 标记扣费失败并终止任务。
func (s *Store) MarkChargeFailed(ctx context.Context, id int64, message string, finishedAt time.Time) (Job, error) {
	if finishedAt.IsZero() {
		finishedAt = time.Now().UTC()
	}
	return s.updateJobReturning(ctx, id, `status = $1, charge_status = $2, charge_message = $3, finished_at = $4`, JobStatusFailed, ChargeStatusFailed, message, finishedAt.UTC())
}

// CancelQueuedJob 只允许取消 queued 任务。
func (s *Store) CancelQueuedJob(ctx context.Context, id int64, finishedAt time.Time) (Job, error) {
	if finishedAt.IsZero() {
		finishedAt = time.Now().UTC()
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE `+s.table("image_generation_jobs")+`
		SET status = $1, finished_at = $2
		WHERE id = $3 AND status = $4
	`, JobStatusCanceled, finishedAt.UTC(), id, JobStatusQueued)
	if err != nil {
		return Job{}, fmt.Errorf("cancel image generation job: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return Job{}, ErrCancelNotAllowed
	}
	return s.GetJob(ctx, id)
}

// MarkRefundSucceeded 标记退款成功。
func (s *Store) MarkRefundSucceeded(ctx context.Context, id int64) (Job, error) {
	return s.updateJobReturning(ctx, id, `charge_status = $1, charge_message = NULL`, ChargeStatusRefunded)
}

// MarkRefundFailed 标记退款失败。
func (s *Store) MarkRefundFailed(ctx context.Context, id int64, message string) (Job, error) {
	return s.updateJobReturning(ctx, id, `charge_status = $1, charge_message = $2`, ChargeStatusRefundFailed, message)
}

// ListQueuedJobs 返回按创建顺序排列的待执行任务。
func (s *Store) ListQueuedJobs(ctx context.Context, limit int) ([]Job, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+jobColumns()+` FROM `+s.table("image_generation_jobs")+`
		WHERE status = $1 AND charge_status IN ($2, $3)
		ORDER BY created_at ASC, id ASC
		LIMIT $4
	`, append([]any{JobStatusQueued}, append(chargeStatusArgs(), limit)...)...)
	if err != nil {
		return nil, fmt.Errorf("query queued image generation jobs: %w", err)
	}
	defer rows.Close()
	return scanJobRows(rows)
}

// ClaimQueuedJob 原子 claim 一个 queued 任务。
func (s *Store) ClaimQueuedJob(ctx context.Context, id int64, startedAt time.Time) (Job, bool, error) {
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE `+s.table("image_generation_jobs")+`
		SET status = $1, started_at = $2
		WHERE id = $3 AND status = $4 AND charge_status IN ($5, $6)
	`, append([]any{JobStatusRunning, startedAt.UTC(), id, JobStatusQueued}, chargeStatusArgs()...)...)
	if err != nil {
		return Job{}, false, fmt.Errorf("claim image generation job: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return Job{}, false, nil
	}
	job, err := s.GetJob(ctx, id)
	return job, err == nil, err
}

// CompleteJob 保存上游结果并切终态。
func (s *Store) CompleteJob(ctx context.Context, id int64, result chatgpt2api.ImageGenerationResponse, finishedAt time.Time) (Job, error) {
	if result.Data == nil {
		result.Data = []chatgpt2api.ImageGenerationData{}
	}
	payload, err := encodeResult(&result)
	if err != nil {
		return Job{}, err
	}
	if finishedAt.IsZero() {
		finishedAt = time.Now().UTC()
	}
	return s.updateJobReturning(ctx, id, `status = $1, result_json = $2::jsonb, error_message = NULL, finished_at = $3`, JobStatusCompleted, payload, finishedAt.UTC())
}

// FailJob 保存失败摘要并切终态。
func (s *Store) FailJob(ctx context.Context, id int64, message string, finishedAt time.Time) (Job, error) {
	if finishedAt.IsZero() {
		finishedAt = time.Now().UTC()
	}
	return s.updateJobReturning(ctx, id, `status = $1, error_message = $2, finished_at = $3`, JobStatusFailed, message, finishedAt.UTC())
}

// RunningCounts 返回平台与用户维度的 running 数量。
func (s *Store) RunningCounts(ctx context.Context) (int, map[int64]int, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT user_id, COUNT(*)
		FROM `+s.table("image_generation_jobs")+`
		WHERE status = $1
		GROUP BY user_id
	`, JobStatusRunning)
	if err != nil {
		return 0, nil, fmt.Errorf("query running image generation counts: %w", err)
	}
	defer rows.Close()
	total := 0
	byUser := map[int64]int{}
	for rows.Next() {
		var userID int64
		var count int
		if err := rows.Scan(&userID, &count); err != nil {
			return 0, nil, err
		}
		byUser[userID] = count
		total += count
	}
	return total, byUser, rows.Err()
}

// RecoverRunningToQueued 将进程重启前残留的 running 任务恢复为 queued。
func (s *Store) RecoverRunningToQueued(ctx context.Context) (int, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE `+s.table("image_generation_jobs")+` SET status = $1, started_at = NULL WHERE status = $2`, JobStatusQueued, JobStatusRunning)
	if err != nil {
		return 0, fmt.Errorf("recover running image generation jobs: %w", err)
	}
	affected, _ := result.RowsAffected()
	return int(affected), nil
}

// CleanupTerminalJobs 删除超过保留期的终态任务。
func (s *Store) CleanupTerminalJobs(ctx context.Context, cutoff time.Time) (int, error) {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM `+s.table("image_generation_jobs")+`
		WHERE status IN ($1, $2, $3) AND finished_at IS NOT NULL AND finished_at < $4
	`, JobStatusCompleted, JobStatusFailed, JobStatusCanceled, cutoff.UTC())
	if err != nil {
		return 0, fmt.Errorf("cleanup image generation jobs: %w", err)
	}
	affected, _ := result.RowsAffected()
	return int(affected), nil
}

func (s *Store) changedUserSession(ctx context.Context, result sql.Result, userID int64, sessionID int64) (Session, error) {
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return Session{}, ErrSessionNotFound
	}
	return s.GetUserSession(ctx, userID, sessionID)
}

func (s *Store) completedJobsForImages(ctx context.Context, userID int64) ([]Job, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+jobColumns()+` FROM `+s.table("image_generation_jobs")+`
		WHERE user_id = $1 AND status = $2 AND result_json IS NOT NULL
		ORDER BY created_at DESC, id DESC
	`, userID, JobStatusCompleted)
	if err != nil {
		return nil, fmt.Errorf("query completed image generation jobs: %w", err)
	}
	defer rows.Close()
	return scanJobRows(rows)
}

func (s *Store) galleryStateForSource(ctx context.Context, taskID int64, imageIndex int) (int64, bool) {
	var id int64
	var visible bool
	err := s.db.QueryRowContext(ctx, `
		SELECT id, is_visible
		FROM `+s.table("image_public_gallery_items")+`
		WHERE source_task_id = $1 AND source_image_index = $2
	`, taskID, imageIndex).Scan(&id, &visible)
	if err != nil {
		return 0, false
	}
	return id, visible
}

func (s *Store) updateJobReturning(ctx context.Context, id int64, setClause string, args ...any) (Job, error) {
	sqlArgs := append(args, id)
	query := `UPDATE ` + s.table("image_generation_jobs") + ` SET ` + setClause + ` WHERE id = $` + fmt.Sprint(len(sqlArgs)) + `
	`
	result, err := s.db.ExecContext(ctx, query, sqlArgs...)
	if err != nil {
		return Job{}, fmt.Errorf("update image generation job: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return Job{}, ErrJobNotFound
	}
	return s.GetJob(ctx, id)
}

func (s *Store) table(name string) string {
	return s.tablePrefix + name
}

func deductUserBalance(ctx context.Context, exec sqlExecutor, userID int64, amount string) error {
	result, err := exec.ExecContext(ctx, `
		UPDATE users
		SET balance = balance - $1::decimal,
		    updated_at = NOW()
		WHERE id = $2
		  AND deleted_at IS NULL
		  AND balance >= $1::decimal
	`, amount, userID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBalanceChargeFailed, err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrInsufficientBalance
	}
	return nil
}

func addUserBalance(ctx context.Context, exec sqlExecutor, userID int64, amount string) error {
	result, err := exec.ExecContext(ctx, `
		UPDATE users
		SET balance = balance + $1::decimal,
		    updated_at = NOW()
		WHERE id = $2
		  AND deleted_at IS NULL
	`, amount, userID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBalanceRefundFailed, err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("%w: user not found", ErrBalanceRefundFailed)
	}
	return nil
}

func insertImageGenerationBalanceHistory(ctx context.Context, exec sqlExecutor, userID int64, code string, note string, value string, at time.Time) error {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	if strings.TrimSpace(code) == "" {
		return fmt.Errorf("image generation balance history code is required")
	}
	if _, err := exec.ExecContext(ctx, `
		INSERT INTO redeem_codes (
			code, type, value, status, used_by, used_at, notes, created_at, validity_days
		) VALUES (
			$1, $2, $3::decimal, $4, $5, $6, $7, $6, 0
		)
	`, code, imageGenerationRedeemType, value, "used", userID, at.UTC(), note); err != nil {
		return fmt.Errorf("insert image generation balance history: %w", err)
	}
	return nil
}

func markRefundPending(ctx context.Context, exec sqlExecutor, tableName string, jobID int64) (bool, error) {
	result, err := exec.ExecContext(ctx, `
		UPDATE `+tableName+`
		SET charge_status = $1, charge_message = NULL
		WHERE id = $2 AND charge_status = $3
	`, ChargeStatusPending, jobID, ChargeStatusSuccess)
	if err != nil {
		return false, fmt.Errorf("mark image generation refund pending: %w", err)
	}
	affected, _ := result.RowsAffected()
	return affected > 0, nil
}

func balanceLifecycleKeySeed(job Job, at time.Time) string {
	return fmt.Sprintf("imagegen:%d:%d:%s:%s:%s:%d:%s:%s", job.UserID, job.SessionID, job.Model, job.Size, job.Quality, job.N, job.Prompt, at.UTC().Format(time.RFC3339Nano))
}

func balanceLifecycleKey(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	// redeem_codes.code 最长 32 字符，保留固定前缀便于从历史记录识别来源。
	return "IG" + strings.ToUpper(hex.EncodeToString(sum[:15]))
}

func validateTablePrefix(prefix string) error {
	for _, r := range strings.TrimSpace(prefix) {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return fmt.Errorf("image generation table prefix is invalid")
	}
	return nil
}

func sessionColumns() string {
	return `id, user_id, username, email, title, title_customized, current_image_task_id, current_image_index, last_task_id, created_at, updated_at, deleted_at`
}

func jobColumns() string {
	return `id, user_id, username, email, status, session_id, generation_mode,
		source_image_task_id, source_image_index, source_image_bytes, source_image_filename,
		source_image_content_type, model, prompt, n, quality, size, publish_to_gallery,
		charge_amount, charge_status, balance_idempotency_key, charge_message,
		result_json, error_message, created_at, started_at, finished_at`
}

func scanSession(row interface{ Scan(dest ...any) error }) (Session, error) {
	session, err := scanSessionAny(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrSessionNotFound
	}
	return session, err
}

func scanSessionRows(row interface{ Scan(dest ...any) error }) (Session, error) {
	return scanSessionAny(row)
}

func scanSessionAny(row interface{ Scan(dest ...any) error }) (Session, error) {
	var session Session
	var username, email sql.NullString
	var titleCustomized sql.NullBool
	var currentTask, lastTask sql.NullInt64
	var currentIndex sql.NullInt64
	var deletedAt sql.NullTime
	err := row.Scan(
		&session.ID, &session.UserID, &username, &email, &session.Title, &titleCustomized,
		&currentTask, &currentIndex, &lastTask, &session.CreatedAt, &session.UpdatedAt, &deletedAt,
	)
	if err != nil {
		return Session{}, err
	}
	session.Username = nullString(username)
	session.Email = nullString(email)
	session.TitleCustomized = titleCustomized.Valid && titleCustomized.Bool
	session.CurrentImageTaskID = nullInt64(currentTask)
	if currentIndex.Valid {
		v := int(currentIndex.Int64)
		session.CurrentImageIndex = &v
	}
	session.LastTaskID = nullInt64(lastTask)
	session.CreatedAt = session.CreatedAt.UTC()
	session.UpdatedAt = session.UpdatedAt.UTC()
	if deletedAt.Valid {
		value := deletedAt.Time.UTC()
		session.DeletedAt = &value
	}
	return session, nil
}

func scanJob(row interface{ Scan(dest ...any) error }) (Job, error) {
	job, err := scanJobAny(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrJobNotFound
	}
	return job, err
}

func scanJobRows(rows *sql.Rows) ([]Job, error) {
	items := []Job{}
	for rows.Next() {
		job, err := scanJobAny(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, job)
	}
	return items, rows.Err()
}

func scanJobAny(row interface{ Scan(dest ...any) error }) (Job, error) {
	var job Job
	var username, email, quality, size, balanceKey, chargeMessage, resultJSON, errorMessage sql.NullString
	var sessionID, sourceTaskID sql.NullInt64
	var sourceIndex sql.NullInt64
	var sourceBytes []byte
	var sourceFilename, sourceContentType sql.NullString
	var startedAt, finishedAt sql.NullTime
	err := row.Scan(
		&job.ID, &job.UserID, &username, &email, &job.Status, &sessionID, &job.GenerationMode,
		&sourceTaskID, &sourceIndex, &sourceBytes, &sourceFilename, &sourceContentType,
		&job.Model, &job.Prompt, &job.N, &quality, &size, &job.PublishToGallery,
		&job.ChargeAmount, &job.ChargeStatus, &balanceKey, &chargeMessage,
		&resultJSON, &errorMessage, &job.CreatedAt, &startedAt, &finishedAt,
	)
	if err != nil {
		return Job{}, err
	}
	job.Username = nullString(username)
	job.Email = nullString(email)
	job.SessionID = nullInt64(sessionID)
	job.SourceImageTaskID = nullInt64(sourceTaskID)
	if sourceIndex.Valid {
		v := int(sourceIndex.Int64)
		job.SourceImageIndex = &v
	}
	job.SourceImageBytes = sourceBytes
	job.SourceImageFilename = nullString(sourceFilename)
	job.SourceImageContentType = nullString(sourceContentType)
	job.Quality = nullString(quality)
	job.Size = nullString(size)
	job.BalanceIdempotencyKey = nullString(balanceKey)
	job.ChargeMessage = nullString(chargeMessage)
	job.ErrorMessage = nullString(errorMessage)
	if resultJSON.Valid && strings.TrimSpace(resultJSON.String) != "" {
		var result chatgpt2api.ImageGenerationResponse
		if err := json.Unmarshal([]byte(resultJSON.String), &result); err != nil {
			return Job{}, fmt.Errorf("decode image generation job result: %w", err)
		}
		if result.Data == nil {
			result.Data = []chatgpt2api.ImageGenerationData{}
		}
		job.Result = &result
	}
	job.CreatedAt = job.CreatedAt.UTC()
	if startedAt.Valid {
		value := startedAt.Time.UTC()
		job.StartedAt = &value
	}
	if finishedAt.Valid {
		value := finishedAt.Time.UTC()
		job.FinishedAt = &value
	}
	return job, nil
}

func encodeResult(result *chatgpt2api.ImageGenerationResponse) (any, error) {
	if result == nil {
		return nil, nil
	}
	if result.Data == nil {
		result.Data = []chatgpt2api.ImageGenerationData{}
	}
	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("encode image generation job result: %w", err)
	}
	return string(data), nil
}

func chargeStatusArgs() []any {
	return []any{ChargeStatusNone, ChargeStatusSuccess}
}

func nullString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func nullInt64(value sql.NullInt64) int64 {
	if !value.Valid {
		return 0
	}
	return value.Int64
}

func nullStringParam(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

func nullInt64Param(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

func nullIntParam(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullBytesParam(value []byte) any {
	if len(value) == 0 {
		return nil
	}
	return value
}

func nullTimeParam(value *time.Time) any {
	if value == nil || value.IsZero() {
		return nil
	}
	return value.UTC()
}

func normalizePageRequest(input PageRequest) PageRequest {
	if input.Page <= 0 {
		input.Page = 1
	}
	if input.PageSize <= 0 || input.PageSize > 20 {
		input.PageSize = 20
	}
	return input
}

func pageCount(total int64, pageSize int) int {
	if pageSize <= 0 || total <= 0 {
		return 0
	}
	return int((total + int64(pageSize) - 1) / int64(pageSize))
}
