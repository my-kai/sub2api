-- 发票管理授权名单；空表表示仅系统管理员可进入发票管理。
CREATE TABLE IF NOT EXISTS custom_invoice_managers (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    granted_by BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_custom_invoice_managers_granted_by
    ON custom_invoice_managers(granted_by);
