package security

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncryptDecryptClaimPayload(t *testing.T) {
	key := DeriveSessionKey([]byte("server-secret"), "ticket-hash", "session-1", "server-nonce", "challenge")
	aad := []byte("activity:1:round:2")

	nonce, ciphertext, err := EncryptClaimPayload(key, []byte(`{"hit_count":3}`), aad)
	require.NoError(t, err)

	plaintext, err := DecryptClaimPayload(key, nonce, ciphertext, aad)
	require.NoError(t, err)
	require.JSONEq(t, `{"hit_count":3}`, string(plaintext))
}

func TestDecryptClaimPayloadRejectsTamperedAAD(t *testing.T) {
	key := DeriveSessionKey([]byte("server-secret"), "ticket-hash", "session-1", "server-nonce", "challenge")
	nonce, ciphertext, err := EncryptClaimPayload(key, []byte(`{"hit_count":3}`), []byte("aad-1"))
	require.NoError(t, err)

	_, err = DecryptClaimPayload(key, nonce, ciphertext, []byte("aad-2"))
	require.ErrorIs(t, err, ErrInvalidSecurityPayload)
}

func TestVerifyClaimSignature(t *testing.T) {
	key := DeriveSessionKey([]byte("server-secret"), "ticket-hash", "session-1", "server-nonce", "challenge")
	signature := SignClaim(key, "session-1", "10", "idem", "nonce", "ciphertext")

	require.True(t, VerifyClaimSignature(key, signature, "session-1", "10", "idem", "nonce", "ciphertext"))
	require.False(t, VerifyClaimSignature(key, signature, "session-1", "10", "idem", "other", "ciphertext"))
}
