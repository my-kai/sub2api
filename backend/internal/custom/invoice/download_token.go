package invoice

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const publicDownloadTokenVersion = 1

type publicDownloadTokenSigner struct {
	secret []byte
	now    func() time.Time
}

type publicDownloadTokenPayload struct {
	Version       int    `json:"v"`
	ApplicationID int64  `json:"aid"`
	ObjectKeyHash string `json:"kh"`
	ExpiresAt     int64  `json:"exp"`
	Nonce         string `json:"n"`
}

func newPublicDownloadTokenSigner(secret string) (*publicDownloadTokenSigner, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, ErrPublicLinkMissing
	}
	return &publicDownloadTokenSigner{secret: []byte(secret), now: func() time.Time { return time.Now().UTC() }}, nil
}

func (s *publicDownloadTokenSigner) issue(app Application, ttl time.Duration) (string, time.Time, error) {
	if s == nil || len(s.secret) == 0 || app.ID <= 0 || strings.TrimSpace(app.FileObjectKey) == "" || ttl <= 0 {
		return "", time.Time{}, ErrPublicLinkMissing
	}
	nonce, err := randomTokenNonce()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("generate invoice download nonce: %w", err)
	}
	expiresAt := s.now().Add(ttl).UTC()
	payload := publicDownloadTokenPayload{
		Version:       publicDownloadTokenVersion,
		ApplicationID: app.ID,
		ObjectKeyHash: hashObjectKey(app.FileObjectKey),
		ExpiresAt:     expiresAt.Unix(),
		Nonce:         nonce,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("marshal invoice download token: %w", err)
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(raw)
	signature := s.sign(encodedPayload)
	return encodedPayload + "." + signature, expiresAt, nil
}

func (s *publicDownloadTokenSigner) verify(token string) (publicDownloadTokenPayload, error) {
	if s == nil || len(s.secret) == 0 {
		return publicDownloadTokenPayload{}, ErrPublicLinkMissing
	}
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return publicDownloadTokenPayload{}, ErrPublicLinkInvalid
	}
	expected := s.sign(parts[0])
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return publicDownloadTokenPayload{}, ErrPublicLinkInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return publicDownloadTokenPayload{}, ErrPublicLinkInvalid
	}
	var payload publicDownloadTokenPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return publicDownloadTokenPayload{}, ErrPublicLinkInvalid
	}
	if payload.Version != publicDownloadTokenVersion || payload.ApplicationID <= 0 || strings.TrimSpace(payload.ObjectKeyHash) == "" {
		return publicDownloadTokenPayload{}, ErrPublicLinkInvalid
	}
	if payload.ExpiresAt <= s.now().Unix() {
		return publicDownloadTokenPayload{}, ErrPublicLinkExpired
	}
	return payload, nil
}

func (s *publicDownloadTokenSigner) sign(encodedPayload string) string {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(encodedPayload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func randomTokenNonce() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashObjectKey(objectKey string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(objectKey)))
	return hex.EncodeToString(sum[:])
}

func linkTokenError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrPublicLinkExpired) || errors.Is(err, ErrPublicLinkInvalid) || errors.Is(err, ErrPublicLinkMissing) {
		return err
	}
	return fmt.Errorf("%w: %v", ErrPublicLinkInvalid, err)
}
