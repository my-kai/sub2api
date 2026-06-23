package oauthapp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	oauth2 "github.com/go-oauth2/oauth2/v4"
	"github.com/go-oauth2/oauth2/v4/manage"
)

// UserReader 定义 OAuth 应用流程只需要的用户读取能力。
type UserReader interface {
	GetByID(ctx context.Context, id int64) (*service.User, error)
}

// TokenIssuer 由 AuthService 实现，用于签发站内正常 token。
type TokenIssuer interface {
	GenerateTokenPair(ctx context.Context, user *service.User, familyID string) (*service.TokenPair, error)
	RecordSuccessfulLogin(ctx context.Context, userID int64)
}

// Service 负责串联应用校验、授权码生成/消费和站内 token 签发。
type Service struct {
	store       *Store
	users       UserReader
	tokenIssuer TokenIssuer
	manager     oauth2.Manager
}

// NewService 装配自定义 OAuth 应用服务。
func NewService(store *Store, users UserReader, tokenIssuer TokenIssuer) *Service {
	manager := manage.NewDefaultManager()
	_ = configureOAuthManager(manager, store)
	return &Service{
		store:       store,
		users:       users,
		tokenIssuer: tokenIssuer,
		manager:     manager,
	}
}

// ListApplications 返回管理员可见的未删除应用列表。
func (s *Service) ListApplications(ctx context.Context) ([]AdminApplication, error) {
	apps, err := s.store.ListApplications(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AdminApplication, 0, len(apps))
	for i := range apps {
		out = append(out, toAdminApplication(&apps[i]))
	}
	return out, nil
}

// CreateApplication 创建 OAuth 客户端，并只在本次响应返回明文密钥。
func (s *Service) CreateApplication(ctx context.Context, req createApplicationRequest) (*AdminApplicationSecret, error) {
	app, secret, err := s.store.CreateApplication(ctx, req.Name, req.AllowedDomains, normalizeStatus(req.Status, ApplicationStatusEnabled))
	if err != nil {
		return nil, err
	}
	return &AdminApplicationSecret{Application: toAdminApplication(app), AccessSecret: secret}, nil
}

// UpdateApplication 更新应用可变配置。
func (s *Service) UpdateApplication(ctx context.Context, id int64, req updateApplicationRequest) (*AdminApplication, error) {
	app, err := s.store.UpdateApplication(ctx, id, req.Name, req.AllowedDomains, normalizeStatus(req.Status, ApplicationStatusEnabled))
	if err != nil {
		return nil, err
	}
	dto := toAdminApplication(app)
	return &dto, nil
}

// ResetSecret 轮换客户端密钥，并只在本次响应返回新明文密钥。
func (s *Service) ResetSecret(ctx context.Context, id int64) (*AdminApplicationSecret, error) {
	app, secret, err := s.store.ResetSecret(ctx, id)
	if err != nil {
		return nil, err
	}
	return &AdminApplicationSecret{Application: toAdminApplication(app), AccessSecret: secret}, nil
}

// DeleteApplication 软删除应用，保留历史记录可追溯。
func (s *Service) DeleteApplication(ctx context.Context, id int64) error {
	return s.store.DeleteApplication(ctx, id)
}

// GetAuthorizeInfo 校验授权请求参数，并返回浏览器授权页展示所需信息。
func (s *Service) GetAuthorizeInfo(ctx context.Context, responseType, clientID, redirectURI, state string) (*AuthorizeInfo, error) {
	app, target, err := s.validateAuthorizeRequest(ctx, responseType, clientID, redirectURI)
	if err != nil {
		return nil, err
	}
	return &AuthorizeInfo{
		ApplicationName: app.Name,
		AccessKey:       app.AccessKey,
		RedirectURI:     target.URL,
		RedirectDomain:  target.Domain,
		State:           strings.TrimSpace(state),
	}, nil
}

// CreateAuthorizationCode 校验用户授权确认，并创建一次性授权码。
func (s *Service) CreateAuthorizationCode(ctx context.Context, userID int64, req AuthorizeConfirmRequest) (*AuthorizeConfirmResponse, error) {
	app, target, err := s.validateAuthorizeRequest(ctx, req.ResponseType, req.ClientID, req.RedirectURI)
	if err != nil {
		return nil, err
	}
	if userID <= 0 {
		return nil, ErrApplicationDenied
	}
	code, err := s.generateLibraryCode(ctx, userID, app.AccessKey, target.URL)
	if err != nil {
		return nil, err
	}
	record, err := s.store.CreateCodeWithValue(ctx, code, userID, app.AccessKey, target.URL, target.Domain)
	if err != nil {
		return nil, err
	}
	redirectURL, err := buildRedirectURL(target.URL, code, req.State)
	if err != nil {
		return nil, err
	}
	return &AuthorizeConfirmResponse{RedirectURL: redirectURL, Code: code, ExpiresAt: record.ExpiresAt}, nil
}

// ExchangeCode 在校验客户端密钥和 redirect_uri 绑定后，消费授权码换取站内 token。
func (s *Service) ExchangeCode(ctx context.Context, grantType, code, clientID, clientSecret, redirectURI string) (*TokenResponse, error) {
	if oauth2.GrantType(strings.TrimSpace(grantType)) != oauth2.AuthorizationCode {
		return nil, ErrApplicationDenied
	}
	app, err := s.validateClientCredentials(ctx, clientID, clientSecret)
	if err != nil {
		return nil, err
	}
	target, err := NormalizeRedirectURI(redirectURI)
	if err != nil {
		return nil, err
	}
	if !IsDomainAllowed(target.Domain, app.AllowedDomains) {
		return nil, ErrApplicationDenied
	}

	record, err := s.store.ConsumeCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if record.AccessKey != app.AccessKey || record.RedirectURI != target.URL {
		return nil, ErrApplicationDenied
	}

	user, err := s.users.GetByID(ctx, record.UserID)
	if err != nil || user == nil || !user.IsActive() {
		return nil, service.ErrInvalidToken
	}
	pair, err := s.tokenIssuer.GenerateTokenPair(ctx, user, "")
	if err != nil {
		return nil, err
	}
	s.tokenIssuer.RecordSuccessfulLogin(ctx, user.ID)
	return &TokenResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    pair.ExpiresIn,
	}, nil
}

func (s *Service) validateAuthorizeRequest(ctx context.Context, responseType, clientID, redirectURI string) (*Application, redirectTarget, error) {
	if strings.TrimSpace(responseType) != "code" {
		return nil, redirectTarget{}, ErrApplicationDenied
	}
	app, err := s.store.GetApplicationByAccessKey(ctx, clientID)
	if err != nil {
		return nil, redirectTarget{}, err
	}
	if !app.Enabled() {
		return nil, redirectTarget{}, ErrApplicationDenied
	}
	target, err := NormalizeRedirectURI(redirectURI)
	if err != nil {
		return nil, redirectTarget{}, err
	}
	if !IsDomainAllowed(target.Domain, app.AllowedDomains) {
		return nil, redirectTarget{}, ErrApplicationDenied
	}
	return app, target, nil
}

func (s *Service) generateLibraryCode(ctx context.Context, userID int64, clientID, redirectURI string) (string, error) {
	if s == nil || s.manager == nil {
		return randomCode()
	}
	token, err := s.manager.GenerateAuthToken(ctx, oauth2.Code, &oauth2.TokenGenerateRequest{
		ClientID:    strings.TrimSpace(clientID),
		UserID:      fmt.Sprintf("%d", userID),
		RedirectURI: strings.TrimSpace(redirectURI),
	})
	if err != nil || token == nil || strings.TrimSpace(token.GetCode()) == "" {
		return "", ErrApplicationDenied
	}
	return token.GetCode(), nil
}

func (s *Service) validateClientCredentials(ctx context.Context, clientID, clientSecret string) (*Application, error) {
	app, err := s.store.GetApplicationByAccessKey(ctx, strings.TrimSpace(clientID))
	if err != nil {
		return nil, err
	}
	if !app.Enabled() {
		return nil, ErrApplicationDenied
	}
	if !compareSecretHash(app.AccessSecretHash, clientSecret) {
		return nil, ErrInvalidSecret
	}
	return app, nil
}

func classifyPublicError(err error) (int, string) {
	switch {
	case errors.Is(err, ErrInvalidRedirectURI), errors.Is(err, ErrInvalidApplication):
		return 400, "应用授权请求无效"
	case errors.Is(err, ErrInvalidSecret), errors.Is(err, ErrApplicationDenied), errors.Is(err, ErrCodeExpired):
		return 401, "应用授权失败"
	default:
		return 500, "授权服务暂不可用"
	}
}
