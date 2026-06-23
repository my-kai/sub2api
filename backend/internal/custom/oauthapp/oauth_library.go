package oauthapp

import (
	"context"
	"strings"
	"sync"

	oauth2 "github.com/go-oauth2/oauth2/v4"
	"github.com/go-oauth2/oauth2/v4/manage"
	"github.com/go-oauth2/oauth2/v4/models"
)

type clientStore struct {
	apps *Store
}

// GetByID 将持久化应用适配成 go-oauth2 需要的 ClientInfo。
// 协议细节交给成熟库处理，但应用密钥仍只以 hash 形式保存在数据库。
func (s clientStore) GetByID(ctx context.Context, id string) (oauth2.ClientInfo, error) {
	if s.apps == nil {
		return nil, ErrInvalidApplication
	}
	app, err := s.apps.GetApplicationByAccessKey(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	if !app.Enabled() {
		return nil, ErrApplicationDenied
	}
	return &models.Client{
		ID:     app.AccessKey,
		Domain: "*",
		Public: false,
	}, nil
}

func configureOAuthManager(manager *manage.Manager, apps *Store) error {
	if manager == nil {
		return nil
	}
	manager.MapClientStorage(clientStore{apps: apps})
	manager.MapTokenStorage(newEphemeralTokenStore())
	manager.SetValidateURIHandler(func(_, redirectURI string) error {
		_, err := NormalizeRedirectURI(redirectURI)
		return err
	})
	return nil
}

type ephemeralTokenStore struct {
	mu     sync.Mutex
	byCode map[string]oauth2.TokenInfo
}

func newEphemeralTokenStore() *ephemeralTokenStore {
	return &ephemeralTokenStore{byCode: map[string]oauth2.TokenInfo{}}
}

// Create 满足 go-oauth2 生成授权码时需要的 token store 契约。
// 实际换取 token 时仍以数据库记录为准，确保授权码只能被消费一次。
func (s *ephemeralTokenStore) Create(_ context.Context, info oauth2.TokenInfo) error {
	if info == nil || strings.TrimSpace(info.GetCode()) == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byCode[info.GetCode()] = info
	return nil
}

func (s *ephemeralTokenStore) RemoveByCode(_ context.Context, code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.byCode, code)
	return nil
}

func (s *ephemeralTokenStore) RemoveByAccess(context.Context, string) error {
	return nil
}

func (s *ephemeralTokenStore) RemoveByRefresh(context.Context, string) error {
	return nil
}

func (s *ephemeralTokenStore) GetByCode(_ context.Context, code string) (oauth2.TokenInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.byCode[code], nil
}

func (s *ephemeralTokenStore) GetByAccess(context.Context, string) (oauth2.TokenInfo, error) {
	return nil, nil
}

func (s *ephemeralTokenStore) GetByRefresh(context.Context, string) (oauth2.TokenInfo, error) {
	return nil, nil
}
