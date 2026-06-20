package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeCallbackAuthAllowedDomains(t *testing.T) {
	got, err := NormalizeCallbackAuthAllowedDomains([]string{
		" https://Example.COM:8443/callback ",
		"*.Sub.Example.COM",
		"localhost",
		"127.0.0.1",
		"EXAMPLE.com",
	})
	require.NoError(t, err)
	require.Equal(t, []string{"example.com", "*.sub.example.com", "localhost", "127.0.0.1"}, got)
}

func TestNormalizeCallbackAuthAllowedDomainsInvalid(t *testing.T) {
	for _, item := range []string{"ftp://example.com/callback", "example.com/path", "*.localhost", "bad_domain"} {
		_, err := NormalizeCallbackAuthAllowedDomains([]string{item})
		require.Error(t, err, item)
	}
}

func TestParseCallbackAuthAllowedDomainsIgnoresInvalidHistoricalValues(t *testing.T) {
	got := ParseCallbackAuthAllowedDomains(`["example.com","bad/path","*.example.com",""]`)
	require.Equal(t, []string{"example.com", "*.example.com"}, got)
}

func TestIsCallbackAuthDomainAllowed(t *testing.T) {
	require.True(t, IsCallbackAuthDomainAllowed("example.com", []string{"example.com"}))
	require.True(t, IsCallbackAuthDomainAllowed("example.com:443", []string{"example.com"}))
	require.True(t, IsCallbackAuthDomainAllowed("app.example.com", []string{"*.example.com"}))
	require.False(t, IsCallbackAuthDomainAllowed("example.com", []string{"*.example.com"}))
	require.False(t, IsCallbackAuthDomainAllowed("evil.com", []string{"example.com"}))
	require.False(t, IsCallbackAuthDomainAllowed("example.com", nil))
}
