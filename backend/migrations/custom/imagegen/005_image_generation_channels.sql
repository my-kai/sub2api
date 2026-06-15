-- Store ordered image generation upstream channels as JSON.
-- Legacy chatgpt2api columns stay in place for rollback and one-time migration.

ALTER TABLE {{TABLE_PREFIX}}image_generation_config
    ADD COLUMN IF NOT EXISTS upstream_channels JSONB NOT NULL DEFAULT '[]'::jsonb;

UPDATE {{TABLE_PREFIX}}image_generation_config
SET upstream_channels = jsonb_build_array(
    jsonb_build_object(
        'id', 'chatgpt2api',
        'name', 'chatgpt2api',
        'type', 'chatgpt2api',
        'enabled', TRUE,
        'base_url', chatgpt2api_base_url,
        'auth_key', chatgpt2api_auth_key,
        'retry_count', 10
    )
)
WHERE upstream_channels = '[]'::jsonb
  AND (BTRIM(chatgpt2api_base_url) <> '' OR BTRIM(chatgpt2api_auth_key) <> '');
