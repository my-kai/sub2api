package imagequeue

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/chatgpt2api"
	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/runtime"
)

func TestServiceSessionsReturnsEmptyArray(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()
	store, err := NewStore(db, "")
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	service := NewService(store)

	mock.ExpectQuery(`SELECT id, user_id, username, email, title, title_customized, current_image_task_id, current_image_index, last_task_id, created_at, updated_at, deleted_at FROM image_generation_sessions`).
		WithArgs(int64(7), 100).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "username", "email", "title", "title_customized", "current_image_task_id", "current_image_index", "last_task_id", "created_at", "updated_at", "deleted_at",
		}))

	items, err := service.Sessions(t.Context(), runtime.UserProfile{ID: 7})
	if err != nil {
		t.Fatalf("Sessions() error = %v", err)
	}
	if items == nil {
		t.Fatalf("Sessions() returned nil slice")
	}
	if len(items) != 0 {
		t.Fatalf("Sessions() = %+v", items)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations: %v", err)
	}
}

func TestCanAccessJobIsolatesUsers(t *testing.T) {
	job := Job{ID: 1, UserID: 7}
	if canAccessJob(runtime.UserProfile{ID: 8, Role: "user"}, job) {
		t.Fatalf("different user should not access job")
	}
	if !canAccessJob(runtime.UserProfile{ID: 7, Role: "user"}, job) {
		t.Fatalf("owner should access job")
	}
	if !canAccessJob(runtime.UserProfile{ID: 99, Role: "admin"}, job) {
		t.Fatalf("admin should access job")
	}
}

func TestIsTerminalCoversStateMachineTerminals(t *testing.T) {
	for _, status := range []JobStatus{JobStatusCompleted, JobStatusFailed, JobStatusCanceled} {
		if !IsTerminal(status) {
			t.Fatalf("%s should be terminal", status)
		}
	}
	for _, status := range []JobStatus{JobStatusQueued, JobStatusRunning} {
		if IsTerminal(status) {
			t.Fatalf("%s should not be terminal", status)
		}
	}
}

func TestShouldPromoteSessionCurrentImageOnlyCompletedWithResult(t *testing.T) {
	if shouldPromoteSessionCurrentImage(Job{Status: JobStatusQueued, SessionID: 1}) {
		t.Fatalf("queued job should not promote current image")
	}
	if shouldPromoteSessionCurrentImage(Job{Status: JobStatusCompleted, SessionID: 0}) {
		t.Fatalf("job without session should not promote current image")
	}
	if !shouldPromoteSessionCurrentImage(Job{
		Status:    JobStatusCompleted,
		SessionID: 1,
		Result:    &chatgpt2api.ImageGenerationResponse{Data: []chatgpt2api.ImageGenerationData{{URL: "https://example.invalid/1.png"}}},
	}) {
		t.Fatalf("completed session job should promote current image")
	}
}

func TestPromptSessionTitleNormalizesAndTruncatesPrompt(t *testing.T) {
	if got := promptSessionTitle("  hello\n  world\tagain  "); got != "hello world again" {
		t.Fatalf("promptSessionTitle() = %q", got)
	}

	longPrompt := strings.Repeat("图", maxSessionTitleLen+10)
	got := promptSessionTitle(longPrompt)
	if len([]rune(got)) != maxSessionTitleLen {
		t.Fatalf("promptSessionTitle length = %d", len([]rune(got)))
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("promptSessionTitle should end with ellipsis: %q", got)
	}
}

func TestServiceConfigHidesChatGPT2APIAuthKey(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()
	store, err := NewStore(db, "")
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	service := NewService(store)
	updatedAt := time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)

	expectGetConfig(mock, configRow{
		PlatformConcurrency:    2,
		DefaultUserConcurrency: 1,
		RetentionDays:          7,
		OneK:                   "0.13400",
		TwoK:                   "0.26800",
		FourK:                  "0.40000",
		BaseURL:                "http://127.0.0.1:8000",
		AuthKey:                "secret-key",
		UpdatedByUserID:        int64Ptr(9),
		UpdatedAt:              updatedAt,
	})

	cfg, err := service.Config(t.Context())
	if err != nil {
		t.Fatalf("Config() error = %v", err)
	}
	if cfg.ChatGPT2API.AuthKey != "" {
		t.Fatalf("AuthKey should be hidden, got %q", cfg.ChatGPT2API.AuthKey)
	}
	if !cfg.ChatGPT2API.AuthKeyConfigured {
		t.Fatalf("AuthKeyConfigured should be true")
	}
	body, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if regexp.MustCompile(`secret-key|auth_key"`).Match(body) {
		t.Fatalf("config JSON leaked auth key: %s", body)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations: %v", err)
	}
}

func TestServiceUpdateConfigKeepsOrClearsChatGPT2APIAuthKey(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()
	store, err := NewStore(db, "")
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	service := NewService(store).WithNow(func() time.Time {
		return time.Date(2026, 6, 14, 11, 0, 0, 0, time.UTC)
	})

	current := configRow{
		PlatformConcurrency:    2,
		DefaultUserConcurrency: 1,
		RetentionDays:          7,
		OneK:                   "0.13400",
		TwoK:                   "0.26800",
		FourK:                  "0.40000",
		BaseURL:                "http://old.local/v1",
		AuthKey:                "old-secret",
		UpdatedByUserID:        int64Ptr(9),
		UpdatedAt:              time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC),
	}
	expectGetConfig(mock, current)
	mock.ExpectExec(`INSERT INTO image_generation_config`).
		WithArgs(true, 3, 2, 9, "0.11100", "0.22200", "0.33300", "http://new.local/v1", "old-secret", int64(10), service.now().UTC()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	cfg, err := service.UpdateConfig(t.Context(), ConfigInput{
		PlatformConcurrency:    3,
		DefaultUserConcurrency: 2,
		RetentionDays:          9,
		UnitPrices:             UnitPriceInput{OneK: "0.111", TwoK: "0.222", FourK: "0.333"},
		ChatGPT2API:            UpstreamConfigInput{BaseURL: "http://new.local/v1"},
	}, runtime.UserProfile{ID: 10})
	if err != nil {
		t.Fatalf("UpdateConfig() keep error = %v", err)
	}
	if cfg.ChatGPT2API.AuthKey != "" || !cfg.ChatGPT2API.AuthKeyConfigured {
		t.Fatalf("sanitized keep config = %+v", cfg.ChatGPT2API)
	}

	expectGetConfig(mock, current)
	mock.ExpectExec(`INSERT INTO image_generation_config`).
		WithArgs(true, 3, 2, 9, "0.11100", "0.22200", "0.33300", "http://new.local/v1", "", int64(10), service.now().UTC()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	cfg, err = service.UpdateConfig(t.Context(), ConfigInput{
		PlatformConcurrency:    3,
		DefaultUserConcurrency: 2,
		RetentionDays:          9,
		UnitPrices:             UnitPriceInput{OneK: "0.111", TwoK: "0.222", FourK: "0.333"},
		ChatGPT2API:            UpstreamConfigInput{BaseURL: "http://new.local/v1", ClearAuthKey: true},
	}, runtime.UserProfile{ID: 10})
	if err != nil {
		t.Fatalf("UpdateConfig() clear error = %v", err)
	}
	if cfg.ChatGPT2API.AuthKeyConfigured {
		t.Fatalf("AuthKeyConfigured should be false after clear")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations: %v", err)
	}
}

func TestServiceUpdateConfigCanDisableImageGeneration(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()
	store, err := NewStore(db, "")
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	service := NewService(store).WithNow(func() time.Time {
		return time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	})

	disabled := false
	expectGetConfig(mock, configRow{
		PlatformConcurrency:    2,
		DefaultUserConcurrency: 1,
		RetentionDays:          7,
		OneK:                   "0.13400",
		TwoK:                   "0.26800",
		FourK:                  "0.40000",
		BaseURL:                "http://old.local/v1",
		AuthKey:                "old-secret",
		UpdatedAt:              time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC),
	})
	mock.ExpectExec(`INSERT INTO image_generation_config`).
		WithArgs(false, 2, 1, 7, "0.13400", "0.26800", "0.40000", "http://old.local/v1", "old-secret", int64(10), service.now().UTC()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	cfg, err := service.UpdateConfig(t.Context(), ConfigInput{
		Enabled:                &disabled,
		PlatformConcurrency:    2,
		DefaultUserConcurrency: 1,
		RetentionDays:          7,
		UnitPrices:             UnitPriceInput{OneK: "0.134", TwoK: "0.268", FourK: "0.4"},
		ChatGPT2API:            UpstreamConfigInput{BaseURL: "http://old.local/v1"},
	}, runtime.UserProfile{ID: 10})
	if err != nil {
		t.Fatalf("UpdateConfig() error = %v", err)
	}
	if cfg.Enabled {
		t.Fatalf("Enabled should be false")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations: %v", err)
	}
}

func TestServiceChatGPT2APIRuntimeConfigUsesStoredConfig(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()
	store, err := NewStore(db, "")
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	service := NewService(store)

	expectGetConfig(mock, configRow{
		PlatformConcurrency:    2,
		DefaultUserConcurrency: 1,
		RetentionDays:          7,
		OneK:                   "0.13400",
		TwoK:                   "0.26800",
		FourK:                  "0.40000",
		BaseURL:                "http://127.0.0.1:8000/v1",
		AuthKey:                "runtime-secret",
		UpdatedAt:              time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC),
	})

	cfg, err := service.ChatGPT2APIRuntimeConfig(t.Context())
	if err != nil {
		t.Fatalf("ChatGPT2APIRuntimeConfig() error = %v", err)
	}
	if cfg.BaseURL.String() != "http://127.0.0.1:8000/v1" || cfg.AuthKey != "runtime-secret" {
		t.Fatalf("runtime config = %+v", cfg)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations: %v", err)
	}
}

func TestServiceCreateTaskChargesBalanceAndCreatesHistory(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()
	store, err := NewStore(db, "")
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	now := time.Date(2026, 6, 14, 13, 0, 0, 0, time.UTC)
	service := NewService(store).WithNow(func() time.Time { return now })
	user := runtime.UserProfile{ID: 7, Username: "demo", Email: "demo@example.com"}

	expectGetConfig(mock, configRow{
		PlatformConcurrency:    2,
		DefaultUserConcurrency: 1,
		RetentionDays:          7,
		OneK:                   "0.13400",
		TwoK:                   "0.26800",
		FourK:                  "0.40000",
		UpdatedAt:              now,
	})
	expectGetUserSession(mock, Session{ID: 11, UserID: 7, Title: "新会话", CreatedAt: now, UpdatedAt: now})
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE users\s+SET balance = balance - \$1::decimal`).
		WithArgs("0.26800", int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`INSERT INTO image_generation_jobs`).
		WithArgs(
			int64(7), "demo", "demo@example.com", JobStatusQueued, int64(11), GenerationModeGenerate,
			nil, nil, nil, nil,
			nil, defaultImageModel, "draw a cat", 2, nil, nil, false,
			"0.26800", ChargeStatusSuccess, sqlmock.AnyArg(), nil,
			nil, nil, now.UTC(), nil, nil,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(21)))
	mock.ExpectExec(`UPDATE image_generation_sessions`).
		WithArgs(int64(21), now.UTC(), int64(11)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO redeem_codes`).
		WithArgs(sqlmock.AnyArg(), imageGenerationRedeemType, "-0.26800", "used", int64(7), now.UTC(), imageGenerationChargeNote).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	expectGetJob(mock, Job{
		ID:               21,
		UserID:           7,
		Username:         "demo",
		Email:            "demo@example.com",
		Status:           JobStatusQueued,
		SessionID:        11,
		GenerationMode:   GenerationModeGenerate,
		Model:            defaultImageModel,
		Prompt:           "draw a cat",
		N:                2,
		ChargeAmount:     "0.26800",
		ChargeStatus:     ChargeStatusSuccess,
		CreatedAt:        now,
		QueuePosition:    1,
		PublishToGallery: false,
	})
	mock.ExpectExec(`UPDATE image_generation_sessions`).
		WithArgs("draw a cat", now.UTC(), int64(11), int64(7), int64(21)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectQueuePosition(mock, 21, now, 1)

	job, err := service.CreateTask(t.Context(), user, CreateJobInput{SessionID: 11, Prompt: "draw a cat", N: 2})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if job.ChargeStatus != ChargeStatusSuccess || job.ChargeAmount != "0.26800" {
		t.Fatalf("charged job = %+v", job)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations: %v", err)
	}
}

func TestServiceCreateTaskRejectsInsufficientBalance(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()
	store, err := NewStore(db, "")
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	now := time.Date(2026, 6, 14, 13, 30, 0, 0, time.UTC)
	service := NewService(store).WithNow(func() time.Time { return now })

	expectGetConfig(mock, configRow{
		PlatformConcurrency:    2,
		DefaultUserConcurrency: 1,
		RetentionDays:          7,
		OneK:                   "0.13400",
		TwoK:                   "0.26800",
		FourK:                  "0.40000",
		UpdatedAt:              now,
	})
	expectGetUserSession(mock, Session{ID: 11, UserID: 7, Title: "新会话", CreatedAt: now, UpdatedAt: now})
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE users\s+SET balance = balance - \$1::decimal`).
		WithArgs("0.13400", int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	_, err = service.CreateTask(t.Context(), runtime.UserProfile{ID: 7}, CreateJobInput{SessionID: 11, Prompt: "draw", N: 1})
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("CreateTask() error = %v, want ErrInsufficientBalance", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations: %v", err)
	}
}

func TestServiceRefundFailedJobIfChargedRefundsBalance(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()
	store, err := NewStore(db, "")
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	now := time.Date(2026, 6, 14, 14, 0, 0, 0, time.UTC)
	service := NewService(store).WithNow(func() time.Time { return now })
	job := Job{
		ID:                    21,
		UserID:                7,
		Status:                JobStatusFailed,
		GenerationMode:        GenerationModeGenerate,
		Model:                 defaultImageModel,
		Prompt:                "draw",
		N:                     1,
		ChargeAmount:          "0.13400",
		ChargeStatus:          ChargeStatusSuccess,
		BalanceIdempotencyKey: "IG123456789012345678901234567890",
		CreatedAt:             now.Add(-time.Minute),
		FinishedAt:            &now,
	}

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE image_generation_jobs\s+SET charge_status = \$1, charge_message = NULL`).
		WithArgs(ChargeStatusPending, int64(21), ChargeStatusSuccess).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE users\s+SET balance = balance \+ \$1::decimal`).
		WithArgs("0.13400", int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO redeem_codes`).
		WithArgs(sqlmock.AnyArg(), imageGenerationRedeemType, "0.13400", "used", int64(7), now.UTC(), imageGenerationRefundNote+"：任务生成失败").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE image_generation_jobs\s+SET charge_status = \$1, charge_message = NULL`).
		WithArgs(ChargeStatusRefunded, int64(21), ChargeStatusSuccess).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	expectGetJob(mock, Job{
		ID:                    21,
		UserID:                7,
		Status:                JobStatusFailed,
		GenerationMode:        GenerationModeGenerate,
		Model:                 defaultImageModel,
		Prompt:                "draw",
		N:                     1,
		ChargeAmount:          "0.13400",
		ChargeStatus:          ChargeStatusRefunded,
		BalanceIdempotencyKey: job.BalanceIdempotencyKey,
		CreatedAt:             now.Add(-time.Minute),
		FinishedAt:            &now,
	})

	refunded, err := service.RefundFailedJobIfCharged(t.Context(), job)
	if err != nil {
		t.Fatalf("RefundFailedJobIfCharged() error = %v", err)
	}
	if refunded.ChargeStatus != ChargeStatusRefunded {
		t.Fatalf("refund status = %s", refunded.ChargeStatus)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations: %v", err)
	}
}

type configRow struct {
	Disabled               bool
	PlatformConcurrency    int
	DefaultUserConcurrency int
	RetentionDays          int
	OneK                   string
	TwoK                   string
	FourK                  string
	BaseURL                string
	AuthKey                string
	UpdatedByUserID        *int64
	UpdatedAt              time.Time
}

func expectGetConfig(mock sqlmock.Sqlmock, row configRow) {
	enabled := !row.Disabled
	rows := sqlmock.NewRows([]string{
		"enabled",
		"platform_concurrency",
		"default_user_concurrency",
		"retention_days",
		"unit_price_low",
		"unit_price_medium",
		"unit_price_high",
		"chatgpt2api_base_url",
		"chatgpt2api_auth_key",
		"updated_by_user_id",
		"updated_at",
	})
	updatedBy := any(nil)
	if row.UpdatedByUserID != nil {
		updatedBy = *row.UpdatedByUserID
	}
	rows.AddRow(enabled, row.PlatformConcurrency, row.DefaultUserConcurrency, row.RetentionDays, row.OneK, row.TwoK, row.FourK, row.BaseURL, row.AuthKey, updatedBy, row.UpdatedAt)
	mock.ExpectQuery(`SELECT enabled, platform_concurrency, default_user_concurrency, retention_days,`).
		WillReturnRows(rows)
}

func expectGetUserSession(mock sqlmock.Sqlmock, session Session) {
	rows := sqlmock.NewRows([]string{
		"id", "user_id", "username", "email", "title", "title_customized", "current_image_task_id", "current_image_index", "last_task_id", "created_at", "updated_at", "deleted_at",
	}).AddRow(session.ID, session.UserID, nil, nil, session.Title, session.TitleCustomized, nil, nil, nullLastTask(session.LastTaskID), session.CreatedAt, session.UpdatedAt, nil)
	mock.ExpectQuery(`SELECT id, user_id, username, email, title, title_customized, current_image_task_id, current_image_index, last_task_id, created_at, updated_at, deleted_at FROM image_generation_sessions`).
		WillReturnRows(rows)
}

func expectGetJob(mock sqlmock.Sqlmock, job Job) {
	rows := sqlmock.NewRows([]string{
		"id", "user_id", "username", "email", "status", "session_id", "generation_mode",
		"source_image_task_id", "source_image_index", "source_image_bytes", "source_image_filename",
		"source_image_content_type", "model", "prompt", "n", "quality", "size", "publish_to_gallery",
		"charge_amount", "charge_status", "balance_idempotency_key", "charge_message",
		"result_json", "error_message", "created_at", "started_at", "finished_at",
	}).AddRow(
		job.ID, job.UserID, nullableString(job.Username), nullableString(job.Email), job.Status, nullableInt64(job.SessionID), job.GenerationMode,
		nil, nil, nil, nil,
		nil, job.Model, job.Prompt, job.N, nullableString(job.Quality), nullableString(job.Size), job.PublishToGallery,
		job.ChargeAmount, job.ChargeStatus, nullableString(job.BalanceIdempotencyKey), nullableString(job.ChargeMessage),
		nil, nullableString(job.ErrorMessage), job.CreatedAt, nullableTime(job.StartedAt), nullableTime(job.FinishedAt),
	)
	mock.ExpectQuery(`SELECT id, user_id, username, email, status, session_id, generation_mode,`).
		WillReturnRows(rows)
}

func expectQueuePosition(mock sqlmock.Sqlmock, jobID int64, createdAt time.Time, position int) {
	expectGetJob(mock, Job{
		ID:             jobID,
		UserID:         7,
		Status:         JobStatusQueued,
		SessionID:      11,
		GenerationMode: GenerationModeGenerate,
		Model:          defaultImageModel,
		Prompt:         "draw a cat",
		N:              2,
		ChargeAmount:   "0.26800",
		ChargeStatus:   ChargeStatusSuccess,
		CreatedAt:      createdAt,
	})
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM image_generation_jobs`).
		WithArgs(JobStatusQueued, createdAt.UTC(), jobID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(position))
}

func nullLastTask(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func nullableInt64(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC()
}

func int64Ptr(value int64) *int64 {
	return &value
}
