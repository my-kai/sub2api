-- custom red packet rain websocket security schema.
-- Stores only hashes and short risk codes; raw tickets, challenges and keys
-- stay transient so audit data cannot be replayed as credentials.

CREATE TABLE IF NOT EXISTS {{TABLE_PREFIX}}custom_red_packet_rain_ws_tickets (
    id BIGSERIAL PRIMARY KEY,
    ticket_hash TEXT NOT NULL UNIQUE,
    activity_id BIGINT NOT NULL
        REFERENCES {{TABLE_PREFIX}}custom_activities(id) ON DELETE CASCADE,
    round_id BIGINT NOT NULL
        REFERENCES {{TABLE_PREFIX}}custom_red_packet_rain_rounds(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL,
    device_fingerprint TEXT NOT NULL,
    client_nonce TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS {{TABLE_PREFIX}}idx_custom_red_packet_rain_ws_tickets_lookup
    ON {{TABLE_PREFIX}}custom_red_packet_rain_ws_tickets (activity_id, round_id, user_id, expires_at DESC);

CREATE TABLE IF NOT EXISTS {{TABLE_PREFIX}}custom_red_packet_rain_ws_sessions (
    id BIGSERIAL PRIMARY KEY,
    session_id TEXT NOT NULL UNIQUE,
    activity_id BIGINT NOT NULL
        REFERENCES {{TABLE_PREFIX}}custom_activities(id) ON DELETE CASCADE,
    round_id BIGINT NOT NULL
        REFERENCES {{TABLE_PREFIX}}custom_red_packet_rain_rounds(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL,
    device_fingerprint TEXT NOT NULL,
    client_nonce TEXT NOT NULL,
    server_nonce TEXT NOT NULL,
    challenge_hash TEXT NOT NULL,
    used_nonces JSONB NOT NULL DEFAULT '[]',
    risk_status TEXT NOT NULL DEFAULT 'ok',
    risk_reason TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    closed_at TIMESTAMPTZ,
    CONSTRAINT {{TABLE_PREFIX}}custom_red_packet_rain_ws_sessions_risk_status_check
        CHECK (risk_status IN ('ok', 'blocked'))
);

CREATE INDEX IF NOT EXISTS {{TABLE_PREFIX}}idx_custom_red_packet_rain_ws_sessions_user_activity
    ON {{TABLE_PREFIX}}custom_red_packet_rain_ws_sessions (user_id, activity_id, expires_at DESC);

CREATE INDEX IF NOT EXISTS {{TABLE_PREFIX}}idx_custom_red_packet_rain_ws_sessions_round
    ON {{TABLE_PREFIX}}custom_red_packet_rain_ws_sessions (activity_id, round_id, expires_at DESC);
