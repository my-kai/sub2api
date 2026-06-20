package callbackauth

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeCallback(t *testing.T) {
	target, err := NormalizeCallback("https://Example.COM:443/path?x=1#fragment")
	require.NoError(t, err)
	require.Equal(t, "example.com", target.Domain)
	require.Equal(t, "https://Example.COM:443/path?x=1", target.URL)
}

func TestNormalizeCallbackRejectsInvalidURLs(t *testing.T) {
	for _, raw := range []string{"", "/local/path", "javascript:alert(1)", "https://user@example.com/cb"} {
		_, err := NormalizeCallback(raw)
		require.ErrorIs(t, err, ErrInvalidCallback, raw)
	}
}

func TestBuildRedirectURLAddsCode(t *testing.T) {
	got, err := buildRedirectURL("https://example.com/cb?state=abc#old", "code-1")
	require.NoError(t, err)
	require.Equal(t, "https://example.com/cb?code=code-1&state=abc", got)
}
