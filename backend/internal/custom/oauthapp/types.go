package oauthapp

import (
	"errors"
	"time"
)

const (
	defaultCodeTTL = 5 * time.Minute
	minCodeTTL     = time.Minute
	maxCodeTTL     = 10 * time.Minute
	codeByteLength = 32
	codePrefix     = "oa_"
	secretPrefix   = "os_"
	keyPrefix      = "ak_"
)

var (
	ErrInvalidApplication = errors.New("oauth application invalid")
	ErrApplicationDenied  = errors.New("oauth application denied")
	ErrInvalidRedirectURI = errors.New("oauth redirect uri invalid")
	ErrInvalidSecret      = errors.New("oauth application secret invalid")
	ErrCodeExpired        = errors.New("oauth authorization code expired")
)

// ApplicationStatus 表示第三方应用当前是否允许发起 OAuth 授权。
type ApplicationStatus string

const (
	ApplicationStatusEnabled  ApplicationStatus = "enabled"
	ApplicationStatusDisabled ApplicationStatus = "disabled"
)

// Application 是第三方 OAuth 应用的内部持久化模型。
type Application struct {
	ID               int64
	Name             string
	AccessKey        string
	AccessSecretHash string
	AllowedDomains   []string
	Status           ApplicationStatus
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time
}

// Enabled 判断应用是否允许签发授权码并换取 token。
func (a *Application) Enabled() bool {
	return a != nil && a.DeletedAt == nil && a.Status == ApplicationStatusEnabled
}

// AdminApplication 是返回给管理页的非敏感应用结构。
type AdminApplication struct {
	ID             int64             `json:"id"`
	Name           string            `json:"name"`
	AccessKey      string            `json:"accessKey"`
	AllowedDomains []string          `json:"allowedDomains"`
	Status         ApplicationStatus `json:"status"`
	CreatedAt      time.Time         `json:"createdAt"`
	UpdatedAt      time.Time         `json:"updatedAt"`
}

// AdminApplicationSecret 只在创建应用或重置密钥后返回。
type AdminApplicationSecret struct {
	Application  AdminApplication `json:"application"`
	AccessSecret string           `json:"accessSecret"`
}

type createApplicationRequest struct {
	Name           string   `json:"name"`
	AllowedDomains []string `json:"allowedDomains"`
	Status         string   `json:"status"`
}

type updateApplicationRequest struct {
	Name           string   `json:"name"`
	AllowedDomains []string `json:"allowedDomains"`
	Status         string   `json:"status"`
}

// AuthorizeInfo 告诉浏览器当前哪个第三方应用正在请求授权。
type AuthorizeInfo struct {
	ApplicationName string `json:"applicationName"`
	AccessKey       string `json:"accessKey"`
	RedirectURI     string `json:"redirectUri"`
	RedirectDomain  string `json:"redirectDomain"`
	State           string `json:"state,omitempty"`
}

// AuthorizeConfirmRequest 表示用户确认授权某个 OAuth 应用。
type AuthorizeConfirmRequest struct {
	ResponseType string `json:"response_type"`
	ClientID     string `json:"client_id"`
	RedirectURI  string `json:"redirect_uri"`
	State        string `json:"state"`
}

// AuthorizeConfirmResponse 包含用户确认授权后的回调目标。
type AuthorizeConfirmResponse struct {
	RedirectURL string    `json:"redirect_url"`
	Code        string    `json:"code"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// TokenResponse 使用 OAuth token 响应字段名，承载站内现有 token。
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
}

type redirectTarget struct {
	URL    string
	Domain string
}

type codeRecord struct {
	UserID         int64
	AccessKey      string
	RedirectURI    string
	RedirectDomain string
	ExpiresAt      time.Time
	CreatedAt      time.Time
}
