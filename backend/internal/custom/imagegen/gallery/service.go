package gallery

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type Service struct {
	store *Store
	now   func() time.Time
}

func NewService(store *Store) *Service {
	return &Service{
		store: store,
		now:   func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) WithNow(now func() time.Time) *Service {
	if now != nil {
		s.now = now
	}
	return s
}

func (s *Service) Publish(ctx context.Context, input UpsertInput) (Item, error) {
	if input.UserID <= 0 || input.SourceTaskID <= 0 || input.SourceImageIndex < 0 || strings.TrimSpace(input.ImageURL) == "" {
		return Item{}, ErrBadInput
	}
	if input.PublishedAt.IsZero() {
		input.PublishedAt = s.now()
	}
	return s.store.UpsertVisible(ctx, input, s.now())
}

func (s *Service) Hide(ctx context.Context, sourceTaskID int64, sourceImageIndex int) (Item, error) {
	if sourceTaskID <= 0 || sourceImageIndex < 0 {
		return Item{}, ErrBadInput
	}
	return s.store.HideBySource(ctx, sourceTaskID, sourceImageIndex, s.now())
}

func (s *Service) VisibleList(ctx context.Context, page int, pageSize int, includePrompt bool) ([]ListItem, int64, error) {
	items, total, err := s.store.ListVisible(ctx, page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	result := make([]ListItem, 0, len(items))
	for _, item := range items {
		listItem := ListItem{
			ID:          item.ID,
			ImageURL:    item.ImageURL,
			PublishedAt: item.PublishedAt,
		}
		if includePrompt {
			listItem.Prompt = item.Prompt
		}
		result = append(result, listItem)
	}
	return result, total, nil
}

func (s *Service) ItemBySource(ctx context.Context, sourceTaskID int64, sourceImageIndex int) (Item, error) {
	item, err := s.store.BySource(ctx, sourceTaskID, sourceImageIndex)
	if err != nil {
		return Item{}, fmt.Errorf("load public gallery item: %w", err)
	}
	return item, nil
}
