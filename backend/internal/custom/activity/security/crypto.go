package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const (
	// SessionKeySize keeps AES-256 available while avoiding custom cipher choices.
	SessionKeySize = 32
	nonceSize     = 12
)

var (
	// ErrInvalidSecurityPayload marks malformed or tampered browser messages.
	ErrInvalidSecurityPayload = errors.New("red packet rain security payload is invalid")
)

// RandomToken returns a URL-safe random string.
//
// It is used for tickets, challenges and nonces so each browser session has
// fresh material that cannot be replayed across rounds.
func RandomToken(byteLength int) (string, error) {
	if byteLength <= 0 {
		byteLength = 32
	}
	buf := make([]byte, byteLength)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// HashToken returns a stable SHA-256 hex digest for values that should not be stored raw.
func HashToken(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}

// DeriveSessionKey derives the per-connection key from server-owned and ticket-bound material.
//
// The resulting key is deterministic for the current session but useless across
// another ticket, challenge or round.
func DeriveSessionKey(secret []byte, ticketHash string, sessionID string, serverNonce string, challenge string) []byte {
	_ = secret // kept for call-site compatibility while the browser derives the same session key.
	sum := sha256.Sum256([]byte(strings.Join([]string{
		strings.TrimSpace(ticketHash),
		strings.TrimSpace(sessionID),
		strings.TrimSpace(serverNonce),
		strings.TrimSpace(challenge),
	}, "\x00")))
	return sum[:SessionKeySize]
}

// SignClaim signs the encrypted claim envelope fields.
func SignClaim(key []byte, parts ...string) string {
	mac := hmac.New(sha256.New, key)
	for _, part := range parts {
		mac.Write([]byte(strings.TrimSpace(part)))
		mac.Write([]byte{0})
	}
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// VerifyClaimSignature checks the HMAC without leaking timing differences.
func VerifyClaimSignature(key []byte, signature string, parts ...string) bool {
	expected, err := base64.RawURLEncoding.DecodeString(SignClaim(key, parts...))
	if err != nil {
		return false
	}
	actual, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(signature))
	if err != nil {
		return false
	}
	return hmac.Equal(actual, expected)
}

// EncryptClaimPayload encrypts JSON claim data for tests and future internal tooling.
func EncryptClaimPayload(key []byte, plaintext []byte, associatedData []byte) (nonce string, ciphertext string, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", "", fmt.Errorf("create claim cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", fmt.Errorf("create claim gcm: %w", err)
	}
	nonceBytes := make([]byte, nonceSize)
	if _, err := rand.Read(nonceBytes); err != nil {
		return "", "", fmt.Errorf("generate claim nonce: %w", err)
	}
	sealed := gcm.Seal(nil, nonceBytes, plaintext, associatedData)
	return base64.RawURLEncoding.EncodeToString(nonceBytes), base64.RawURLEncoding.EncodeToString(sealed), nil
}

// DecryptClaimPayload decrypts a browser claim payload.
func DecryptClaimPayload(key []byte, nonce string, ciphertext string, associatedData []byte) ([]byte, error) {
	nonceBytes, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(nonce))
	if err != nil {
		return nil, ErrInvalidSecurityPayload
	}
	ciphertextBytes, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(ciphertext))
	if err != nil {
		return nil, ErrInvalidSecurityPayload
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create claim cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create claim gcm: %w", err)
	}
	if len(nonceBytes) != gcm.NonceSize() {
		return nil, ErrInvalidSecurityPayload
	}
	plaintext, err := gcm.Open(nil, nonceBytes, ciphertextBytes, associatedData)
	if err != nil {
		return nil, ErrInvalidSecurityPayload
	}
	return plaintext, nil
}
