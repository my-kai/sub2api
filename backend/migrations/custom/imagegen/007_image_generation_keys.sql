-- Store user-owned image generation API keys for OpenAI-compatible custom endpoints.
-- Full plaintext keys are returned only at creation time; this table stores lookup hash
-- plus display fragments for safe UI rendering.

CREATE TABLE IF NOT EXISTS {{TABLE_PREFIX}}image_generation_api_keys (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    name TEXT NOT NULL,
    key_prefix TEXT NOT NULL,
    key_suffix TEXT NOT NULL DEFAULT '',
    key_hash TEXT NOT NULL UNIQUE,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    deleted_at TIMESTAMPTZ,
    CONSTRAINT {{TABLE_PREFIX}}image_generation_api_keys_name_check
        CHECK (BTRIM(name) <> ''),
    CONSTRAINT {{TABLE_PREFIX}}image_generation_api_keys_key_prefix_check
        CHECK (BTRIM(key_prefix) <> ''),
    CONSTRAINT {{TABLE_PREFIX}}image_generation_api_keys_key_hash_check
        CHECK (BTRIM(key_hash) <> '')
);

CREATE INDEX IF NOT EXISTS {{TABLE_PREFIX}}idx_image_generation_api_keys_user_created
    ON {{TABLE_PREFIX}}image_generation_api_keys (user_id, created_at DESC, id DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS {{TABLE_PREFIX}}idx_image_generation_api_keys_hash_active
    ON {{TABLE_PREFIX}}image_generation_api_keys (key_hash)
    WHERE deleted_at IS NULL AND enabled = TRUE;
