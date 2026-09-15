CREATE TABLE IF NOT EXISTS custom_prompt_audit_v2_configs (
    id SMALLINT PRIMARY KEY CHECK (id = 1),
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    prompt_template TEXT NOT NULL DEFAULT '',
    worker_count INTEGER NOT NULL DEFAULT 0 CHECK (worker_count BETWEEN 0 AND 64),
    queue_capacity INTEGER NOT NULL DEFAULT 0 CHECK (queue_capacity BETWEEN 0 AND 10000),
    enabled_protocols JSONB NOT NULL DEFAULT '[]'::jsonb,
    log_retention_days INTEGER NOT NULL DEFAULT 0 CHECK (log_retention_days BETWEEN 0 AND 3650),
    config_version BIGINT NOT NULL DEFAULT 1 CHECK (config_version > 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by BIGINT
);

INSERT INTO custom_prompt_audit_v2_configs (id)
VALUES (1)
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS custom_prompt_audit_v2_endpoints (
    id VARCHAR(64) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    base_url TEXT NOT NULL,
    api_key_ciphertext TEXT NOT NULL,
    model VARCHAR(160) NOT NULL,
    priority INTEGER NOT NULL CHECK (priority BETWEEN -100000 AND 100000),
    timeout_ms INTEGER NOT NULL CHECK (timeout_ms BETWEEN 100 AND 120000),
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    config_order INTEGER NOT NULL CHECK (config_order >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_custom_prompt_audit_v2_endpoints_order
    ON custom_prompt_audit_v2_endpoints (enabled, priority DESC, config_order ASC);

CREATE TABLE IF NOT EXISTS custom_prompt_audit_v2_rules (
    id VARCHAR(64) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    confidence_threshold DOUBLE PRECISION NOT NULL CHECK (confidence_threshold BETWEEN 0 AND 1),
    window_minutes INTEGER NOT NULL CHECK (window_minutes BETWEEN 1 AND 10080),
    trigger_count INTEGER NOT NULL CHECK (trigger_count BETWEEN 1 AND 10000),
    action VARCHAR(16) NOT NULL CHECK (action IN ('ban', 'warning')),
    restriction_minutes INTEGER,
    config_order INTEGER NOT NULL CHECK (config_order >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT custom_prompt_audit_v2_rules_action_fields CHECK (
        (action = 'ban' AND restriction_minutes IS NULL) OR
        (action = 'warning' AND restriction_minutes BETWEEN 1 AND 43200)
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_custom_prompt_audit_v2_rules_threshold
    ON custom_prompt_audit_v2_rules (confidence_threshold);

CREATE TABLE IF NOT EXISTS custom_prompt_audit_v2_events (
    id BIGSERIAL PRIMARY KEY,
    request_id VARCHAR(160) NOT NULL,
    user_id BIGINT NOT NULL,
    username_snapshot VARCHAR(255) NOT NULL DEFAULT '',
    user_email_snapshot VARCHAR(255) NOT NULL DEFAULT '',
    api_key_id BIGINT NOT NULL DEFAULT 0,
    api_key_name_snapshot VARCHAR(255) NOT NULL DEFAULT '',
    protocol VARCHAR(64) NOT NULL,
    request_model VARCHAR(255) NOT NULL DEFAULT '',
    endpoint_id VARCHAR(64) NOT NULL,
    endpoint_name_snapshot VARCHAR(100) NOT NULL,
    audit_model VARCHAR(160) NOT NULL,
    confidence DOUBLE PRECISION NOT NULL CHECK (confidence BETWEEN 0 AND 1),
    reason TEXT NOT NULL,
    latency_ms INTEGER NOT NULL CHECK (latency_ms >= 0),
    rule_id VARCHAR(64) NOT NULL,
    rule_name_snapshot VARCHAR(100) NOT NULL,
    rule_threshold DOUBLE PRECISION NOT NULL CHECK (rule_threshold BETWEEN 0 AND 1),
    rule_window_minutes INTEGER NOT NULL,
    rule_trigger_count INTEGER NOT NULL,
    rule_action VARCHAR(16) NOT NULL,
    rule_restriction_minutes INTEGER,
    window_hit_count INTEGER NOT NULL CHECK (window_hit_count > 0),
    threshold_reached BOOLEAN NOT NULL DEFAULT FALSE,
    final_action VARCHAR(16) NOT NULL,
    action_result VARCHAR(32) NOT NULL,
    message_ciphertext TEXT NOT NULL,
    message_sha256 CHAR(64) NOT NULL,
    message_chars INTEGER NOT NULL CHECK (message_chars >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_custom_prompt_audit_v2_events_user_rule_time
    ON custom_prompt_audit_v2_events (user_id, rule_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_custom_prompt_audit_v2_events_user_time
    ON custom_prompt_audit_v2_events (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_custom_prompt_audit_v2_events_rule_time
    ON custom_prompt_audit_v2_events (rule_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_custom_prompt_audit_v2_events_created_at
    ON custom_prompt_audit_v2_events (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_custom_prompt_audit_v2_events_protocol
    ON custom_prompt_audit_v2_events (protocol, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_custom_prompt_audit_v2_events_action
    ON custom_prompt_audit_v2_events (final_action, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_custom_prompt_audit_v2_events_confidence
    ON custom_prompt_audit_v2_events (confidence, created_at DESC);

CREATE TABLE IF NOT EXISTS custom_prompt_audit_v2_restrictions (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    event_id BIGINT REFERENCES custom_prompt_audit_v2_events(id) ON DELETE SET NULL,
    rule_id VARCHAR(64) NOT NULL,
    action VARCHAR(16) NOT NULL CHECK (action IN ('ban', 'warning')),
    started_at TIMESTAMPTZ NOT NULL,
    blocked_until TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT custom_prompt_audit_v2_restrictions_action_fields CHECK (
        (action = 'ban' AND blocked_until IS NULL) OR
        (action = 'warning' AND blocked_until IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_custom_prompt_audit_v2_restrictions_active
    ON custom_prompt_audit_v2_restrictions (user_id, action, blocked_until);

CREATE TABLE IF NOT EXISTS custom_prompt_audit_v2_email_jobs (
    id BIGSERIAL PRIMARY KEY,
    event_id BIGINT NOT NULL UNIQUE REFERENCES custom_prompt_audit_v2_events(id) ON DELETE CASCADE,
    recipient_email VARCHAR(255) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'sent', 'failed')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_until TIMESTAMPTZ,
    last_error_code VARCHAR(100) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sent_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_custom_prompt_audit_v2_email_jobs_claim
    ON custom_prompt_audit_v2_email_jobs (status, available_at, lease_until, id);
