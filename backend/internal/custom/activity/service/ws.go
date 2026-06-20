package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/custom/activity/security"
	"github.com/Wei-Shaw/sub2api/internal/custom/activity/types"
)

const (
	defaultWSTicketTTL       = 30 * time.Second
	defaultWSSessionTTL      = 2 * time.Minute
	maxTraceDigestLength     = 256
	maxDeviceFingerprintSize = 256
)

// WSTicketInput contains the browser entry data for one red packet rain socket.
type WSTicketInput struct {
	ActivityID        int64
	RoundID           int64
	UserID            int64
	DeviceFingerprint string
	ClientNonce       string
	ClientIP          string
	UserAgent         string
}

// WSTicketResult returns the raw ticket once to the browser.
type WSTicketResult struct {
	Ticket    string    `json:"ticket"`
	ExpiresAt time.Time `json:"expires_at"`
	WSURL     string    `json:"ws_url"`
}

// WSChallengeInput is the service request after the WebSocket handshake passes JWT auth.
type WSChallengeInput struct {
	ActivityID int64
	Ticket     string
}

// WSChallengeResult contains the challenge message plus server-only session key material.
type WSChallengeResult struct {
	SessionID   string
	ServerNonce string
	Challenge   string
	ExpiresAt   time.Time
	UserID      int64
	RoundID     int64
	RoundEndsAt time.Time
	Key         []byte
}

// WSClaimEnvelope is the encrypted message envelope received over WebSocket.
type WSClaimEnvelope struct {
	SessionID      string `json:"session_id"`
	RoundID        int64  `json:"round_id"`
	IdempotencyKey string `json:"idempotency_key"`
	Nonce          string `json:"nonce"`
	Ciphertext     string `json:"ciphertext"`
	Signature      string `json:"signature"`
}

// WSClaimPayload is the encrypted browser claim payload.
type WSClaimPayload struct {
	HitCount          int       `json:"hit_count"`
	StartedAt         time.Time `json:"started_at"`
	EndedAt           time.Time `json:"ended_at"`
	ClickTraceDigest  string    `json:"click_trace_digest"`
	DeviceFingerprint string    `json:"device_fingerprint"`
	ClientNonce       string    `json:"client_nonce"`
}

// IssueRedPacketRainWSTicket validates round availability and creates a short-lived ticket.
func (s *Service) IssueRedPacketRainWSTicket(ctx context.Context, input WSTicketInput) (WSTicketResult, error) {
	if s == nil || s.store == nil {
		return WSTicketResult{}, fmt.Errorf("custom activity service is not configured")
	}
	input.DeviceFingerprint = strings.TrimSpace(input.DeviceFingerprint)
	input.ClientNonce = strings.TrimSpace(input.ClientNonce)
	if input.ActivityID <= 0 || input.RoundID <= 0 || input.UserID <= 0 ||
		input.DeviceFingerprint == "" || input.ClientNonce == "" ||
		len(input.DeviceFingerprint) > maxDeviceFingerprintSize {
		return WSTicketResult{}, types.ErrInvalidInput
	}
	if !s.allowActivitySecurityEvent("ticket:user", strconv.FormatInt(input.UserID, 10), 12, time.Minute) ||
		!s.allowActivitySecurityEvent("ticket:device", input.DeviceFingerprint, 20, time.Minute) ||
		!s.allowActivitySecurityEvent("ticket:ip", input.ClientIP, 60, time.Minute) {
		return WSTicketResult{}, types.ErrRedPacketRainSecurityRejected
	}

	activity, _, rounds, err := s.loadActivityConfigRounds(ctx, input.ActivityID)
	if err != nil {
		return WSTicketResult{}, err
	}
	now := s.now().UTC()
	if err := validateClaimWindow(activity, rounds, input.RoundID, now); err != nil {
		return WSTicketResult{}, err
	}

	rawTicket, err := security.RandomToken(32)
	if err != nil {
		return WSTicketResult{}, err
	}
	expiresAt := now.Add(defaultWSTicketTTL)
	if _, err := s.store.CreateWSTicket(ctx, types.RedPacketRainWSTicket{
		TicketHash:        security.HashToken(rawTicket),
		ActivityID:        input.ActivityID,
		RoundID:           input.RoundID,
		UserID:            input.UserID,
		DeviceFingerprint: input.DeviceFingerprint,
		ClientNonce:       input.ClientNonce,
		ExpiresAt:         expiresAt,
		CreatedAt:         now,
	}); err != nil {
		return WSTicketResult{}, err
	}
	return WSTicketResult{
		Ticket:    rawTicket,
		ExpiresAt: expiresAt,
		WSURL:     fmt.Sprintf("/api/v1/custom/activities/%d/red-packet-rain/ws", input.ActivityID),
	}, nil
}

// OpenRedPacketRainWSSession consumes a ticket and creates the socket challenge.
func (s *Service) OpenRedPacketRainWSSession(ctx context.Context, input WSChallengeInput) (WSChallengeResult, error) {
	if s == nil || s.store == nil {
		return WSChallengeResult{}, fmt.Errorf("custom activity service is not configured")
	}
	if input.ActivityID <= 0 || strings.TrimSpace(input.Ticket) == "" {
		return WSChallengeResult{}, types.ErrInvalidInput
	}
	now := s.now().UTC()
	ticket, err := s.store.ConsumeWSTicket(ctx, security.HashToken(input.Ticket), now)
	if err != nil {
		return WSChallengeResult{}, err
	}
	if ticket.ActivityID != input.ActivityID {
		return WSChallengeResult{}, types.ErrRedPacketRainSecurityRejected
	}
	if !s.allowActivitySecurityEvent("ws:user", strconv.FormatInt(ticket.UserID, 10), 30, time.Minute) {
		return WSChallengeResult{}, types.ErrRedPacketRainSecurityRejected
	}
	activity, _, rounds, err := s.loadActivityConfigRounds(ctx, input.ActivityID)
	if err != nil {
		return WSChallengeResult{}, err
	}
	if err := validateClaimWindow(activity, rounds, ticket.RoundID, now); err != nil {
		return WSChallengeResult{}, err
	}
	roundEndsAt := activity.EndsAt
	for _, round := range rounds {
		if round.ID == ticket.RoundID {
			roundEndsAt = round.EndsAt
			break
		}
	}

	sessionID, err := security.RandomToken(24)
	if err != nil {
		return WSChallengeResult{}, err
	}
	serverNonce, err := security.RandomToken(24)
	if err != nil {
		return WSChallengeResult{}, err
	}
	challenge, err := security.RandomToken(32)
	if err != nil {
		return WSChallengeResult{}, err
	}
	expiresAt := minTime(now.Add(defaultWSSessionTTL), roundEndsAt)
	key := security.DeriveSessionKey([]byte(ticket.TicketHash), ticket.TicketHash, sessionID, serverNonce, challenge)
	if _, err := s.store.CreateWSSession(ctx, types.RedPacketRainWSSession{
		SessionID:         sessionID,
		ActivityID:        ticket.ActivityID,
		RoundID:           ticket.RoundID,
		UserID:            ticket.UserID,
		DeviceFingerprint: ticket.DeviceFingerprint,
		ClientNonce:       ticket.ClientNonce,
		ServerNonce:       serverNonce,
		ChallengeHash:     security.HashToken(challenge),
		UsedNonces:        []string{},
		RiskStatus:        "ok",
		ExpiresAt:         expiresAt,
		CreatedAt:         now,
	}); err != nil {
		return WSChallengeResult{}, err
	}
	return WSChallengeResult{
		SessionID:   sessionID,
		ServerNonce: serverNonce,
		Challenge:   challenge,
		ExpiresAt:   expiresAt,
		UserID:      ticket.UserID,
		RoundID:     ticket.RoundID,
		RoundEndsAt: roundEndsAt,
		Key:         key,
	}, nil
}

// ClaimRedPacketRainFromWS validates encrypted WebSocket security data before settlement.
func (s *Service) ClaimRedPacketRainFromWS(ctx context.Context, userID int64, envelope WSClaimEnvelope, sessionKey []byte) (types.ClaimResult, error) {
	if s == nil || s.store == nil {
		return types.ClaimResult{}, fmt.Errorf("custom activity service is not configured")
	}
	if userID <= 0 || strings.TrimSpace(envelope.SessionID) == "" || envelope.RoundID <= 0 ||
		strings.TrimSpace(envelope.IdempotencyKey) == "" || strings.TrimSpace(envelope.Nonce) == "" ||
		strings.TrimSpace(envelope.Ciphertext) == "" || strings.TrimSpace(envelope.Signature) == "" {
		return types.ClaimResult{}, types.ErrInvalidInput
	}
	if !security.VerifyClaimSignature(sessionKey, envelope.Signature, envelope.SessionID,
		strconv.FormatInt(envelope.RoundID, 10), envelope.IdempotencyKey, envelope.Nonce, envelope.Ciphertext) {
		return types.ClaimResult{}, types.ErrRedPacketRainSecurityRejected
	}
	if err := s.store.MarkWSSessionNonceUsed(ctx, envelope.SessionID, envelope.Nonce); err != nil {
		return types.ClaimResult{}, err
	}
	session, err := s.store.GetWSSession(ctx, envelope.SessionID)
	if err != nil {
		return types.ClaimResult{}, err
	}
	if session.UserID != userID || session.RoundID != envelope.RoundID || session.ClosedAt != nil ||
		session.RiskStatus == "blocked" || !s.now().UTC().Before(session.ExpiresAt) {
		return types.ClaimResult{}, types.ErrRedPacketRainSecurityRejected
	}

	plaintext, err := security.DecryptClaimPayload(sessionKey, envelope.Nonce, envelope.Ciphertext, claimAssociatedData(envelope))
	if err != nil {
		return types.ClaimResult{}, types.ErrRedPacketRainSecurityRejected
	}
	var payload WSClaimPayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return types.ClaimResult{}, types.ErrInvalidInput
	}
	if err := s.validateWSClaimPayload(session, payload); err != nil {
		_ = s.store.MarkWSSessionRisk(ctx, envelope.SessionID, "blocked", "invalid_payload")
		return types.ClaimResult{}, err
	}
	if !s.allowActivitySecurityEvent("claim:user", strconv.FormatInt(userID, 10), 8, time.Minute) ||
		!s.allowActivitySecurityEvent("claim:device", session.DeviceFingerprint, 12, time.Minute) {
		_ = s.store.MarkWSSessionRisk(ctx, envelope.SessionID, "blocked", "rate_limited")
		return types.ClaimResult{}, types.ErrRedPacketRainSecurityRejected
	}
	return s.ClaimRedPacketRain(ctx, ClaimInput{
		ActivityID:     session.ActivityID,
		RoundID:        session.RoundID,
		UserID:         session.UserID,
		HitCount:       payload.HitCount,
		IdempotencyKey: envelope.IdempotencyKey,
	})
}

// CloseRedPacketRainWSSession marks one socket session closed.
func (s *Service) CloseRedPacketRainWSSession(ctx context.Context, sessionID string) {
	if s == nil || s.store == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	_ = s.store.CloseWSSession(ctx, sessionID, s.now().UTC())
}

func (s *Service) validateWSClaimPayload(session types.RedPacketRainWSSession, payload WSClaimPayload) error {
	if payload.HitCount < 0 {
		return types.ErrInvalidInput
	}
	if strings.TrimSpace(payload.DeviceFingerprint) != session.DeviceFingerprint ||
		strings.TrimSpace(payload.ClientNonce) != session.ClientNonce {
		return types.ErrRedPacketRainSecurityRejected
	}
	if payload.HitCount > 0 && strings.TrimSpace(payload.ClickTraceDigest) == "" {
		return types.ErrRedPacketRainSecurityRejected
	}
	if len(strings.TrimSpace(payload.ClickTraceDigest)) > maxTraceDigestLength {
		return types.ErrInvalidInput
	}
	if !payload.StartedAt.IsZero() && !payload.EndedAt.IsZero() && payload.EndedAt.Before(payload.StartedAt) {
		return types.ErrRedPacketRainSecurityRejected
	}
	return nil
}

func (s *Service) allowActivitySecurityEvent(scope string, key string, limit int, window time.Duration) bool {
	if s == nil || s.rateLimiter == nil {
		return true
	}
	return s.rateLimiter.Allow(scope, key, security.RateLimitRule{Limit: limit, Window: window})
}

func claimAssociatedData(envelope WSClaimEnvelope) []byte {
	return []byte(strings.Join([]string{
		strings.TrimSpace(envelope.SessionID),
		strconv.FormatInt(envelope.RoundID, 10),
		strings.TrimSpace(envelope.IdempotencyKey),
	}, ":"))
}

func minTime(a time.Time, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
