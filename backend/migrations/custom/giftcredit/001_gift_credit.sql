CREATE TABLE IF NOT EXISTS {{TABLE_PREFIX}}custom_gift_credit_grants (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    source_type TEXT NOT NULL,
    source_id TEXT NOT NULL DEFAULT '',
    original_amount DECIMAL(20,8) NOT NULL,
    remaining_amount DECIMAL(20,8) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    note TEXT NOT NULL DEFAULT '',
    created_by BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT {{TABLE_PREFIX}}custom_gift_credit_grants_original_positive CHECK (original_amount > 0),
    CONSTRAINT {{TABLE_PREFIX}}custom_gift_credit_grants_remaining_nonnegative CHECK (remaining_amount >= 0),
    CONSTRAINT {{TABLE_PREFIX}}custom_gift_credit_grants_remaining_lte_original CHECK (remaining_amount <= original_amount),
    CONSTRAINT {{TABLE_PREFIX}}custom_gift_credit_grants_expiry_after_create CHECK (expires_at > created_at),
    CONSTRAINT {{TABLE_PREFIX}}custom_gift_credit_grants_status_valid CHECK (status IN ('active', 'depleted', 'expired'))
);

CREATE INDEX IF NOT EXISTS {{TABLE_PREFIX}}idx_gift_credit_grants_user_expiry
    ON {{TABLE_PREFIX}}custom_gift_credit_grants (user_id, expires_at, id)
    WHERE remaining_amount > 0 AND status = 'active';

CREATE INDEX IF NOT EXISTS {{TABLE_PREFIX}}idx_gift_credit_grants_user_status_expiry
    ON {{TABLE_PREFIX}}custom_gift_credit_grants (user_id, status, expires_at);

CREATE INDEX IF NOT EXISTS {{TABLE_PREFIX}}idx_gift_credit_grants_source
    ON {{TABLE_PREFIX}}custom_gift_credit_grants (source_type, source_id);

CREATE TABLE IF NOT EXISTS {{TABLE_PREFIX}}custom_gift_credit_deductions (
    id BIGSERIAL PRIMARY KEY,
    grant_id BIGINT NOT NULL REFERENCES {{TABLE_PREFIX}}custom_gift_credit_grants(id),
    user_id BIGINT NOT NULL,
    request_id TEXT NOT NULL,
    usage_billing_key TEXT NOT NULL DEFAULT '',
    amount DECIMAL(20,8) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT {{TABLE_PREFIX}}custom_gift_credit_deductions_amount_positive CHECK (amount > 0)
);

CREATE INDEX IF NOT EXISTS {{TABLE_PREFIX}}idx_gift_credit_deductions_user_created
    ON {{TABLE_PREFIX}}custom_gift_credit_deductions (user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS {{TABLE_PREFIX}}idx_gift_credit_deductions_request
    ON {{TABLE_PREFIX}}custom_gift_credit_deductions (request_id);

CREATE TABLE IF NOT EXISTS {{TABLE_PREFIX}}custom_gift_credit_user_balances (
    user_id BIGINT PRIMARY KEY,
    active_remaining_amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    next_expires_at TIMESTAMPTZ,
    refreshed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT {{TABLE_PREFIX}}custom_gift_credit_user_balances_amount_nonnegative CHECK (active_remaining_amount >= 0)
);
