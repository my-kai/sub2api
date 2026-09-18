CREATE TABLE IF NOT EXISTS custom_turn_log_configs (
    id SMALLINT PRIMARY KEY CHECK (id = 1),
    retention_days INTEGER NOT NULL DEFAULT 30 CHECK (retention_days BETWEEN 1 AND 3650),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by BIGINT
);

INSERT INTO custom_turn_log_configs (id)
VALUES (1)
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS custom_turn_logs (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL,
    account_name VARCHAR(255) NOT NULL DEFAULT '',
    status_code INTEGER NOT NULL CHECK (status_code IN (217, 292)),
    response_headers JSONB NOT NULL DEFAULT '{}'::jsonb,
    response_body TEXT NOT NULL DEFAULT '',
    headers_truncated BOOLEAN NOT NULL DEFAULT FALSE,
    body_truncated BOOLEAN NOT NULL DEFAULT FALSE,
    body_complete BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_custom_turn_logs_created_at
    ON custom_turn_logs (created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_custom_turn_logs_account_time
    ON custom_turn_logs (account_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_custom_turn_logs_status_time
    ON custom_turn_logs (status_code, created_at DESC, id DESC);
