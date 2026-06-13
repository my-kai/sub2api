package gallery

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestVisibleListHidesPromptWhenAnonymous(t *testing.T) {
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

	publishedAt := time.Unix(1, 0).UTC()
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM image_public_gallery_items WHERE is_visible = TRUE`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT id, user_id, source_task_id, source_image_index, image_url, prompt`).
		WithArgs(20, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "source_task_id", "source_image_index", "image_url", "prompt",
			"is_visible", "created_from_public_generation", "published_at", "hidden_at", "created_at", "updated_at",
		}).AddRow(1, 7, 9, 0, "https://example.invalid/1.png", "secret", true, false, publishedAt, nil, publishedAt, publishedAt))

	items, total, err := service.VisibleList(t.Context(), 1, 20, false)
	if err != nil {
		t.Fatalf("VisibleList() error = %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("items=%+v total=%d", items, total)
	}
	if items[0].Prompt != "" {
		t.Fatalf("anonymous prompt = %q", items[0].Prompt)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations: %v", err)
	}
}

func TestPublishAndHideValidateGalleryInput(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()
	store, err := NewStore(db, "")
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	service := NewService(store)
	if _, err := service.Publish(t.Context(), UpsertInput{}); err != ErrBadInput {
		t.Fatalf("Publish() err = %v", err)
	}
	if _, err := service.Hide(t.Context(), 0, 0); err != ErrBadInput {
		t.Fatalf("Hide() err = %v", err)
	}
}
