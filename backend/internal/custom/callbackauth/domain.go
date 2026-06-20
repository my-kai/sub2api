package callbackauth

import (
	"net/url"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// NormalizeCallback validates a browser-supplied callback URL and returns the
// URL without fragment plus a canonical hostname for allowlist and consent keys.
func NormalizeCallback(raw string) (callbackTarget, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return callbackTarget{}, ErrInvalidCallback
	}

	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return callbackTarget{}, ErrInvalidCallback
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return callbackTarget{}, ErrInvalidCallback
	}
	if parsed.User != nil {
		return callbackTarget{}, ErrInvalidCallback
	}

	domain, err := service.NormalizeCallbackAuthHostname(parsed.Host)
	if err != nil {
		return callbackTarget{}, ErrInvalidCallback
	}
	parsed.Fragment = ""
	return callbackTarget{
		URL:    parsed.String(),
		Domain: domain,
	}, nil
}

func buildRedirectURL(callbackURL string, code string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(callbackURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", ErrInvalidCallback
	}
	values := parsed.Query()
	values.Set("code", code)
	parsed.RawQuery = values.Encode()
	parsed.Fragment = ""
	return parsed.String(), nil
}
