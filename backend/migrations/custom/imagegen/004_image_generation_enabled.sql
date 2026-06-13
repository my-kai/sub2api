-- Add a global switch for custom image generation.
-- Default TRUE keeps existing deployments enabled after the custom migration runs.

ALTER TABLE {{TABLE_PREFIX}}image_generation_config
    ADD COLUMN IF NOT EXISTS enabled BOOLEAN NOT NULL DEFAULT TRUE;
