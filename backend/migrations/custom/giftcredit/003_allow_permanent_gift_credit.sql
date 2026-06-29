ALTER TABLE {{TABLE_PREFIX}}custom_gift_credit_grants
    DROP CONSTRAINT IF EXISTS {{TABLE_PREFIX}}custom_gift_credit_grants_expiry_after_create;

ALTER TABLE {{TABLE_PREFIX}}custom_gift_credit_grants
    ALTER COLUMN expires_at DROP NOT NULL;

ALTER TABLE {{TABLE_PREFIX}}custom_gift_credit_grants
    ADD CONSTRAINT {{TABLE_PREFIX}}custom_gift_credit_grants_expiry_after_create
    CHECK (expires_at IS NULL OR expires_at > created_at);
