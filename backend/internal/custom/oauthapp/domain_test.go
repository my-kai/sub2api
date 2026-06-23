package oauthapp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeRedirectURI(t *testing.T) {
	target, err := NormalizeRedirectURI("https://App.Example.COM:443/callback?x=1#fragment")
	require.NoError(t, err)
	require.Equal(t, "app.example.com", target.Domain)
	require.Equal(t, "https://App.Example.COM:443/callback?x=1", target.URL)
}

func TestNormalizeRedirectURIRejectsUnsafeValues(t *testing.T) {
	for _, raw := range []string{"", "/local", "javascript:alert(1)", "https://user@example.com/cb"} {
		_, err := NormalizeRedirectURI(raw)
		require.ErrorIs(t, err, ErrInvalidRedirectURI, raw)
	}
}

func TestNormalizeAllowedDomains(t *testing.T) {
	got, err := NormalizeAllowedDomains([]string{"*.Example.com", "app.example.com", "app.example.com"})
	require.NoError(t, err)
	require.Equal(t, []string{"*.example.com", "app.example.com"}, got)
}

func TestNormalizeAllowedDomainsRejectsInvalidWildcards(t *testing.T) {
	for _, raw := range []string{"*", "bad.*.example.com", "https://example.com", "*.127.0.0.1"} {
		_, err := NormalizeAllowedDomains([]string{raw})
		require.ErrorIs(t, err, ErrInvalidRedirectURI, raw)
	}
}

func TestIsDomainAllowed(t *testing.T) {
	allowlist := []string{"example.com", "*.trusted.example"}
	require.True(t, IsDomainAllowed("example.com", allowlist))
	require.True(t, IsDomainAllowed("app.trusted.example", allowlist))
	require.True(t, IsDomainAllowed("deep.app.trusted.example", allowlist))
	require.False(t, IsDomainAllowed("trusted.example", allowlist), "wildcard does not match bare domain")
	require.False(t, IsDomainAllowed("badexample.com", allowlist))
}

func TestBuildRedirectURLAddsCodeAndState(t *testing.T) {
	got, err := buildRedirectURL("https://example.com/cb?x=1#old", "code-1", "state-1")
	require.NoError(t, err)
	require.Equal(t, "https://example.com/cb?code=code-1&state=state-1&x=1", got)
}

func TestCompareSecretHash(t *testing.T) {
	hash, err := hashSecret("secret-value")
	require.NoError(t, err)
	require.True(t, compareSecretHash(hash, "secret-value"))
	require.False(t, compareSecretHash(hash, "other-secret"))
}
