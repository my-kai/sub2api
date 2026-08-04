CREATE TABLE custom_ai_gateway_admin_transfers (
    transfer_id VARCHAR(128) PRIMARY KEY,
    user_id BIGINT NOT NULL,
    amount_usd NUMERIC(18,6) NOT NULL,
    status VARCHAR(32) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT custom_ai_gateway_admin_transfers_amount_valid CHECK (amount_usd > 0),
    CONSTRAINT custom_ai_gateway_admin_transfers_status_valid CHECK (status = 'debited')
);

CREATE INDEX custom_ai_gateway_admin_transfers_user_created_idx
    ON custom_ai_gateway_admin_transfers (user_id, created_at DESC);
