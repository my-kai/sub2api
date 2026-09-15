-- API keys are re-entered by an administrator after the release. The old
-- ciphertext is intentionally not copied because its encryption key may no
-- longer match the running service.
ALTER TABLE custom_prompt_audit_v2_endpoints
    ADD COLUMN IF NOT EXISTS api_key TEXT NOT NULL DEFAULT '';

-- Existing enabled endpoints intentionally remain enabled but have no key
-- until an administrator re-enters credentials. Runtime activation validates
-- the empty key and fails closed without preventing the main service startup.

ALTER TABLE custom_prompt_audit_v2_endpoints
    DROP COLUMN IF EXISTS api_key_ciphertext;
