package oauthapp

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net"
	"net/url"
	"slices"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"golang.org/x/crypto/bcrypt"
)

// NormalizeRedirectURI 校验 OAuth redirect_uri，并返回去除 fragment 后的回调地址。
// 同时返回规范化域名，供应用白名单匹配使用。
func NormalizeRedirectURI(raw string) (redirectTarget, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return redirectTarget{}, ErrInvalidRedirectURI
	}

	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return redirectTarget{}, ErrInvalidRedirectURI
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return redirectTarget{}, ErrInvalidRedirectURI
	}
	if parsed.User != nil {
		return redirectTarget{}, ErrInvalidRedirectURI
	}

	domain, err := service.NormalizeCallbackAuthHostname(parsed.Host)
	if err != nil {
		return redirectTarget{}, ErrInvalidRedirectURI
	}
	parsed.Fragment = ""
	return redirectTarget{URL: parsed.String(), Domain: domain}, nil
}

// NormalizeAllowedDomains 规范化应用白名单域名。
// 这里会拒绝存在歧义的通配写法，避免 redirect_uri 匹配结果不可预测。
func NormalizeAllowedDomains(domains []string) ([]string, error) {
	normalized := make([]string, 0, len(domains))
	seen := map[string]struct{}{}
	for _, raw := range domains {
		domain, err := normalizeAllowedDomain(raw)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[domain]; ok {
			continue
		}
		seen[domain] = struct{}{}
		normalized = append(normalized, domain)
	}
	slices.Sort(normalized)
	if len(normalized) == 0 {
		return nil, ErrInvalidRedirectURI
	}
	return normalized, nil
}

func normalizeAllowedDomain(raw string) (string, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return "", ErrInvalidRedirectURI
	}
	if strings.Contains(raw, "://") || strings.ContainsAny(raw, "/?#@") {
		return "", ErrInvalidRedirectURI
	}

	if strings.HasPrefix(raw, "*.") {
		base, err := service.NormalizeCallbackAuthHostname(strings.TrimPrefix(raw, "*."))
		if err != nil || base == "" {
			return "", ErrInvalidRedirectURI
		}
		if net.ParseIP(base) != nil {
			return "", ErrInvalidRedirectURI
		}
		return "*." + base, nil
	}
	if strings.Contains(raw, "*") {
		return "", ErrInvalidRedirectURI
	}

	domain, err := service.NormalizeCallbackAuthHostname(raw)
	if err != nil || domain == "" {
		return "", ErrInvalidRedirectURI
	}
	return domain, nil
}

// IsDomainAllowed 按精确域名和通配子域名规则判断是否允许回调。
// 类似 *.example.com 的通配规则只匹配子域名，不匹配 example.com 裸域。
func IsDomainAllowed(domain string, allowlist []string) bool {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" {
		return false
	}
	for _, item := range allowlist {
		item = strings.TrimSpace(strings.ToLower(item))
		if item == "" {
			continue
		}
		if strings.HasPrefix(item, "*.") {
			base := strings.TrimPrefix(item, "*.")
			if domain != base && strings.HasSuffix(domain, "."+base) {
				return true
			}
			continue
		}
		if domain == item {
			return true
		}
	}
	return false
}

func buildRedirectURL(redirectURI, code, state string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(redirectURI))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", ErrInvalidRedirectURI
	}
	values := parsed.Query()
	values.Set("code", code)
	if strings.TrimSpace(state) != "" {
		values.Set("state", strings.TrimSpace(state))
	}
	parsed.RawQuery = values.Encode()
	parsed.Fragment = ""
	return parsed.String(), nil
}

func randomAccessKey() (string, error) {
	value, err := randomToken(24)
	if err != nil {
		return "", err
	}
	return keyPrefix + value, nil
}

func randomSecret() (string, error) {
	value, err := randomToken(32)
	if err != nil {
		return "", err
	}
	return secretPrefix + value, nil
}

func randomCode() (string, error) {
	value, err := randomToken(codeByteLength)
	if err != nil {
		return "", err
	}
	return codePrefix + value, nil
}

func randomToken(byteLength int) (string, error) {
	raw := make([]byte, byteLength)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func hashCode(code string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(code)))
	return hex.EncodeToString(sum[:])
}

func hashSecret(secret string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(strings.TrimSpace(secret)), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

func compareSecretHash(hash, secret string) bool {
	hash = strings.TrimSpace(hash)
	secret = strings.TrimSpace(secret)
	if hash == "" || secret == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(secret)) == nil
}

func normalizeStatus(raw string, fallback ApplicationStatus) ApplicationStatus {
	switch ApplicationStatus(strings.TrimSpace(strings.ToLower(raw))) {
	case ApplicationStatusEnabled:
		return ApplicationStatusEnabled
	case ApplicationStatusDisabled:
		return ApplicationStatusDisabled
	default:
		if fallback != "" {
			return fallback
		}
		return ApplicationStatusEnabled
	}
}

func toAdminApplication(app *Application) AdminApplication {
	if app == nil {
		return AdminApplication{}
	}
	return AdminApplication{
		ID:             app.ID,
		Name:           app.Name,
		AccessKey:      app.AccessKey,
		AllowedDomains: append([]string(nil), app.AllowedDomains...),
		Status:         app.Status,
		CreatedAt:      app.CreatedAt,
		UpdatedAt:      app.UpdatedAt,
	}
}
