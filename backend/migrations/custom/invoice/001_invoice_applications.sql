CREATE TABLE IF NOT EXISTS custom_invoice_titles (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    company_title TEXT NOT NULL,
    tax_number TEXT NOT NULL,
    receiver_email TEXT NOT NULL,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    deleted_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT custom_invoice_titles_company_title_required CHECK (company_title <> ''),
    CONSTRAINT custom_invoice_titles_tax_number_required CHECK (tax_number <> ''),
    CONSTRAINT custom_invoice_titles_receiver_email_required CHECK (receiver_email <> '')
);

CREATE INDEX IF NOT EXISTS idx_custom_invoice_titles_user_deleted_created
    ON custom_invoice_titles(user_id, deleted_at, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_custom_invoice_titles_user_default
    ON custom_invoice_titles(user_id, is_default);

CREATE UNIQUE INDEX IF NOT EXISTS idx_custom_invoice_titles_one_default
    ON custom_invoice_titles(user_id)
    WHERE is_default = TRUE AND deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS custom_invoice_applications (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    invoice_type TEXT NOT NULL DEFAULT 'enterprise_vat_normal',
    title_id BIGINT NULL REFERENCES custom_invoice_titles(id) ON DELETE SET NULL,
    company_title TEXT NOT NULL,
    tax_number TEXT NOT NULL,
    receiver_email TEXT NOT NULL,
    total_amount DECIMAL(20,8) NOT NULL,
    currency TEXT NOT NULL DEFAULT 'CNY',
    order_count INT NOT NULL,
    invoice_number TEXT NOT NULL DEFAULT '',
    admin_remark TEXT NOT NULL DEFAULT '',
    reject_reason TEXT NOT NULL DEFAULT '',
    file_object_key TEXT NOT NULL DEFAULT '',
    file_original_name TEXT NOT NULL DEFAULT '',
    file_size BIGINT NOT NULL DEFAULT 0,
    issued_by BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
    issued_at TIMESTAMPTZ NULL,
    rejected_by BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
    rejected_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT custom_invoice_applications_status_valid CHECK (status IN ('pending', 'issued', 'rejected')),
    CONSTRAINT custom_invoice_applications_type_valid CHECK (invoice_type = 'enterprise_vat_normal'),
    CONSTRAINT custom_invoice_applications_total_amount_positive CHECK (total_amount > 0),
    CONSTRAINT custom_invoice_applications_order_count_positive CHECK (order_count > 0),
    CONSTRAINT custom_invoice_applications_company_title_required CHECK (company_title <> ''),
    CONSTRAINT custom_invoice_applications_tax_number_required CHECK (tax_number <> ''),
    CONSTRAINT custom_invoice_applications_receiver_email_required CHECK (receiver_email <> ''),
    CONSTRAINT custom_invoice_applications_issued_complete CHECK (
        status <> 'issued'
        OR (invoice_number <> '' AND admin_remark <> '' AND file_object_key <> '' AND issued_at IS NOT NULL)
    ),
    CONSTRAINT custom_invoice_applications_rejected_complete CHECK (
        status <> 'rejected'
        OR (reject_reason <> '' AND rejected_at IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_custom_invoice_applications_user_created
    ON custom_invoice_applications(user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_custom_invoice_applications_status_created
    ON custom_invoice_applications(status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_custom_invoice_applications_created
    ON custom_invoice_applications(created_at DESC);

CREATE TABLE IF NOT EXISTS custom_invoice_application_orders (
    application_id BIGINT NOT NULL REFERENCES custom_invoice_applications(id) ON DELETE CASCADE,
    order_id BIGINT NOT NULL REFERENCES payment_orders(id),
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount DECIMAL(20,8) NOT NULL,
    currency TEXT NOT NULL DEFAULT 'CNY',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (application_id, order_id),
    CONSTRAINT custom_invoice_application_orders_amount_positive CHECK (amount > 0)
);

CREATE INDEX IF NOT EXISTS idx_custom_invoice_application_orders_order
    ON custom_invoice_application_orders(order_id);

CREATE INDEX IF NOT EXISTS idx_custom_invoice_application_orders_user_order
    ON custom_invoice_application_orders(user_id, order_id);
