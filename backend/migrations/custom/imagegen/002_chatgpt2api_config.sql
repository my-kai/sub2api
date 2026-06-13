-- Store chatgpt2api upstream configuration in the custom image generation config row.
-- The auth key stays server-side and is never returned by the admin config API.

ALTER TABLE {{TABLE_PREFIX}}image_generation_config
    ADD COLUMN IF NOT EXISTS chatgpt2api_base_url TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS chatgpt2api_auth_key TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS chatgpt2api_env_seeded BOOLEAN NOT NULL DEFAULT FALSE;
