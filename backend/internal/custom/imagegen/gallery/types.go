package gallery

import (
	"errors"
	"time"
)

var (
	ErrNotFound  = errors.New("gallery item not found")
	ErrForbidden = errors.New("gallery access is forbidden")
	ErrBadInput  = errors.New("gallery input is invalid")
)

// Item 保存单张图片在公开展厅里的独立记录。
type Item struct {
	ID                          int64      `json:"id"`
	UserID                      int64      `json:"user_id,omitempty"`
	SourceTaskID                int64      `json:"source_task_id,omitempty"`
	SourceImageIndex            int        `json:"source_image_index,omitempty"`
	ImageURL                    string     `json:"image_url"`
	Prompt                      string     `json:"prompt,omitempty"`
	IsVisible                   bool       `json:"is_visible"`
	CreatedFromPublicGeneration bool       `json:"created_from_public_generation,omitempty"`
	PublishedAt                 time.Time  `json:"published_at"`
	HiddenAt                    *time.Time `json:"hidden_at,omitempty"`
	CreatedAt                   time.Time  `json:"created_at"`
	UpdatedAt                   time.Time  `json:"updated_at"`
}

// UpsertInput 定义单张图片加入展厅或恢复显示时需要固化的快照。
type UpsertInput struct {
	UserID                      int64
	SourceTaskID                int64
	SourceImageIndex            int
	ImageURL                    string
	Prompt                      string
	CreatedFromPublicGeneration bool
	PublishedAt                 time.Time
}

// ListItem 是公开展厅分页接口返回的单条记录。
type ListItem struct {
	ID          int64     `json:"id"`
	ImageURL    string    `json:"image_url"`
	PublishedAt time.Time `json:"published_at"`
	Prompt      string    `json:"prompt,omitempty"`
}
