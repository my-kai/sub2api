package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/custom/activity/types"
)

// CreateWSTicket stores a one-time WebSocket entry ticket hash.
func (s *Store) CreateWSTicket(ctx context.Context, ticket types.RedPacketRainWSTicket) (types.RedPacketRainWSTicket, error) {
	if s == nil || s.db == nil || strings.TrimSpace(ticket.TicketHash) == "" || ticket.ActivityID <= 0 || ticket.RoundID <= 0 || ticket.UserID <= 0 {
		return types.RedPacketRainWSTicket{}, types.ErrInvalidInput
	}
	row := s.db.QueryRowContext(ctx, `
		INSERT INTO `+s.table("custom_red_packet_rain_ws_tickets")+` (
			ticket_hash, activity_id, round_id, user_id, device_fingerprint, client_nonce, expires_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING `+wsTicketColumns()+`
	`, strings.TrimSpace(ticket.TicketHash), ticket.ActivityID, ticket.RoundID, ticket.UserID,
		strings.TrimSpace(ticket.DeviceFingerprint), strings.TrimSpace(ticket.ClientNonce),
		ticket.ExpiresAt.UTC(), normalizeNow(ticket.CreatedAt))
	stored, err := scanWSTicket(row)
	if err != nil {
		return types.RedPacketRainWSTicket{}, fmt.Errorf("create red packet rain websocket ticket: %w", err)
	}
	return stored, nil
}

// ConsumeWSTicket marks an unused ticket as consumed and returns its bound state.
func (s *Store) ConsumeWSTicket(ctx context.Context, ticketHash string, now time.Time) (types.RedPacketRainWSTicket, error) {
	if s == nil || s.db == nil || strings.TrimSpace(ticketHash) == "" {
		return types.RedPacketRainWSTicket{}, types.ErrInvalidInput
	}
	row := s.db.QueryRowContext(ctx, `
		UPDATE `+s.table("custom_red_packet_rain_ws_tickets")+`
		SET consumed_at = $2
		WHERE ticket_hash = $1
		  AND consumed_at IS NULL
		  AND expires_at > $2
		RETURNING `+wsTicketColumns()+`
	`, strings.TrimSpace(ticketHash), normalizeNow(now))
	ticket, err := scanWSTicket(row)
	if errors.Is(err, sql.ErrNoRows) {
		return types.RedPacketRainWSTicket{}, types.ErrRedPacketRainSecurityRejected
	}
	if err != nil {
		return types.RedPacketRainWSTicket{}, fmt.Errorf("consume red packet rain websocket ticket: %w", err)
	}
	return ticket, nil
}

// CreateWSSession stores the challenge state for one WebSocket claim session.
func (s *Store) CreateWSSession(ctx context.Context, session types.RedPacketRainWSSession) (types.RedPacketRainWSSession, error) {
	if s == nil || s.db == nil || strings.TrimSpace(session.SessionID) == "" || session.ActivityID <= 0 || session.RoundID <= 0 || session.UserID <= 0 {
		return types.RedPacketRainWSSession{}, types.ErrInvalidInput
	}
	usedNonces, err := json.Marshal(session.UsedNonces)
	if err != nil {
		return types.RedPacketRainWSSession{}, fmt.Errorf("encode websocket used nonces: %w", err)
	}
	row := s.db.QueryRowContext(ctx, `
		INSERT INTO `+s.table("custom_red_packet_rain_ws_sessions")+` (
			session_id, activity_id, round_id, user_id, device_fingerprint, client_nonce, server_nonce,
			challenge_hash, used_nonces, risk_status, risk_reason, expires_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10, $11, $12, $13)
		RETURNING `+wsSessionColumns()+`
	`, strings.TrimSpace(session.SessionID), session.ActivityID, session.RoundID, session.UserID,
		strings.TrimSpace(session.DeviceFingerprint), strings.TrimSpace(session.ClientNonce), strings.TrimSpace(session.ServerNonce),
		strings.TrimSpace(session.ChallengeHash), string(usedNonces), defaultString(session.RiskStatus, "ok"),
		strings.TrimSpace(session.RiskReason), session.ExpiresAt.UTC(), normalizeNow(session.CreatedAt))
	stored, err := scanWSSession(row)
	if err != nil {
		return types.RedPacketRainWSSession{}, fmt.Errorf("create red packet rain websocket session: %w", err)
	}
	return stored, nil
}

// GetWSSession returns an active WebSocket session by public session id.
func (s *Store) GetWSSession(ctx context.Context, sessionID string) (types.RedPacketRainWSSession, error) {
	if s == nil || s.db == nil || strings.TrimSpace(sessionID) == "" {
		return types.RedPacketRainWSSession{}, types.ErrInvalidInput
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT `+wsSessionColumns()+`
		FROM `+s.table("custom_red_packet_rain_ws_sessions")+`
		WHERE session_id = $1
	`, strings.TrimSpace(sessionID))
	session, err := scanWSSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return types.RedPacketRainWSSession{}, types.ErrNotFound
	}
	if err != nil {
		return types.RedPacketRainWSSession{}, fmt.Errorf("query red packet rain websocket session: %w", err)
	}
	return session, nil
}

// MarkWSSessionNonceUsed atomically records a claim nonce for replay protection.
func (s *Store) MarkWSSessionNonceUsed(ctx context.Context, sessionID string, nonce string) error {
	if s == nil || s.db == nil || strings.TrimSpace(sessionID) == "" || strings.TrimSpace(nonce) == "" {
		return types.ErrInvalidInput
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE `+s.table("custom_red_packet_rain_ws_sessions")+`
		SET used_nonces = used_nonces || to_jsonb($2::text)
		WHERE session_id = $1
		  AND NOT (used_nonces ? $2)
	`, strings.TrimSpace(sessionID), strings.TrimSpace(nonce))
	if err != nil {
		return fmt.Errorf("mark red packet rain websocket nonce: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return types.ErrRedPacketRainSecurityRejected
	}
	return nil
}

// MarkWSSessionRisk stores a short risk code without exposing internal details to clients.
func (s *Store) MarkWSSessionRisk(ctx context.Context, sessionID string, status string, reason string) error {
	if s == nil || s.db == nil || strings.TrimSpace(sessionID) == "" {
		return types.ErrInvalidInput
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE `+s.table("custom_red_packet_rain_ws_sessions")+`
		SET risk_status = $2,
		    risk_reason = $3
		WHERE session_id = $1
	`, strings.TrimSpace(sessionID), defaultString(status, "blocked"), strings.TrimSpace(reason))
	if err != nil {
		return fmt.Errorf("mark red packet rain websocket risk: %w", err)
	}
	return nil
}

// CloseWSSession marks a WebSocket session closed so later cleanup can ignore it.
func (s *Store) CloseWSSession(ctx context.Context, sessionID string, closedAt time.Time) error {
	if s == nil || s.db == nil || strings.TrimSpace(sessionID) == "" {
		return types.ErrInvalidInput
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE `+s.table("custom_red_packet_rain_ws_sessions")+`
		SET closed_at = $2
		WHERE session_id = $1
		  AND closed_at IS NULL
	`, strings.TrimSpace(sessionID), normalizeNow(closedAt))
	if err != nil {
		return fmt.Errorf("close red packet rain websocket session: %w", err)
	}
	return nil
}
