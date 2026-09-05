ALTER TABLE custom_invoice_applications
    ADD COLUMN IF NOT EXISTS created_by BIGINT NULL REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_custom_invoice_applications_created_by
    ON custom_invoice_applications(created_by, created_at DESC);
