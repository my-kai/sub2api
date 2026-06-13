package imagequeue

import (
	"encoding/json"
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

func int64Ptr(value int64) *int64 {
	return &value
}
