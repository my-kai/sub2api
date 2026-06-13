-- custom image generation schema.
-- This file is intentionally outside backend/migrations/*.sql; the main runner does not load it recursively.

CREATE TABLE IF NOT EXISTS {{TABLE_PREFIX}}image_generation_config (
    id BIGINT PRIMARY KEY CHECK (id = 1),
    platform_concurrency INTEGER NOT NULL CHECK (platform_concurrency >= 1),
    default_user_concurrency INTEGER NOT NULL CHECK (default_user_concurrency >= 1),
    retention_days INTEGER NOT NULL CHECK (retention_days >= 1),
    unit_price_auto TEXT NOT NULL DEFAULT '0.13400',
    unit_price_low TEXT NOT NULL DEFAULT '0.13400',
    unit_price_medium TEXT NOT NULL DEFAULT '0.26800',
    unit_price_high TEXT NOT NULL DEFAULT '0.40000',
    updated_by_user_id BIGINT,
    updated_at TIMESTAMPTZ NOT NULL
);

INSERT INTO {{TABLE_PREFIX}}image_generation_config (
    id,
    platform_concurrency,
    default_user_concurrency,
    retention_days,
    unit_price_auto,
    unit_price_low,
    unit_price_medium,
    unit_price_high,
    updated_by_user_id,
    updated_at
)
VALUES (1, 2, 1, 7, '0.13400', '0.13400', '0.26800', '0.40000', NULL, NOW())
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS {{TABLE_PREFIX}}image_generation_user_limits (
    user_id BIGINT PRIMARY KEY,
    username TEXT,
    email TEXT,
    concurrency INTEGER NOT NULL CHECK (concurrency >= 1),
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS {{TABLE_PREFIX}}image_generation_sessions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    username TEXT,
    email TEXT,
    title TEXT NOT NULL,
    current_image_task_id BIGINT,
    current_image_index INTEGER,
    last_task_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    deleted_at TIMESTAMPTZ,
    CONSTRAINT {{TABLE_PREFIX}}image_generation_sessions_current_image_index_check
        CHECK (current_image_index IS NULL OR current_image_index >= 0)
);

CREATE INDEX IF NOT EXISTS {{TABLE_PREFIX}}idx_image_generation_sessions_user_updated
    ON {{TABLE_PREFIX}}image_generation_sessions (user_id, updated_at DESC, id DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS {{TABLE_PREFIX}}idx_image_generation_sessions_deleted
    ON {{TABLE_PREFIX}}image_generation_sessions (deleted_at)
    WHERE deleted_at IS NOT NULL;

CREATE TABLE IF NOT EXISTS {{TABLE_PREFIX}}image_generation_jobs (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    username TEXT,
    email TEXT,
    status TEXT NOT NULL CHECK (status IN ('queued', 'running', 'completed', 'failed', 'canceled')),
    session_id BIGINT,
    generation_mode TEXT NOT NULL DEFAULT 'generate',
    source_image_task_id BIGINT,
    source_image_index INTEGER,
    source_image_bytes BYTEA,
    source_image_filename TEXT,
    source_image_content_type TEXT,
    model TEXT NOT NULL,
    prompt TEXT NOT NULL,
    n INTEGER NOT NULL CHECK (n >= 1),
    quality TEXT,
    size TEXT,
    publish_to_gallery BOOLEAN NOT NULL DEFAULT FALSE,
    charge_amount TEXT NOT NULL DEFAULT '0',
    charge_status TEXT NOT NULL DEFAULT 'none',
    balance_idempotency_key TEXT,
    charge_message TEXT,
    result_json JSONB,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    CONSTRAINT {{TABLE_PREFIX}}image_generation_jobs_generation_mode_check
        CHECK (generation_mode IN ('generate', 'edit')),
    CONSTRAINT {{TABLE_PREFIX}}image_generation_jobs_source_image_index_check
        CHECK (source_image_index IS NULL OR source_image_index >= 0),
    CONSTRAINT {{TABLE_PREFIX}}image_generation_jobs_charge_status_check
        CHECK (charge_status IN ('none', 'pending', 'success', 'failed', 'refunded', 'refund_failed'))
);

CREATE INDEX IF NOT EXISTS {{TABLE_PREFIX}}idx_image_generation_jobs_queue
    ON {{TABLE_PREFIX}}image_generation_jobs (status, created_at ASC, id ASC);

CREATE INDEX IF NOT EXISTS {{TABLE_PREFIX}}idx_image_generation_jobs_user_status
    ON {{TABLE_PREFIX}}image_generation_jobs (user_id, status, created_at ASC);

CREATE INDEX IF NOT EXISTS {{TABLE_PREFIX}}idx_image_generation_jobs_cleanup
    ON {{TABLE_PREFIX}}image_generation_jobs (status, finished_at);

CREATE INDEX IF NOT EXISTS {{TABLE_PREFIX}}idx_image_generation_jobs_session_created
    ON {{TABLE_PREFIX}}image_generation_jobs (session_id, created_at DESC, id DESC)
    WHERE session_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS {{TABLE_PREFIX}}idx_image_generation_jobs_source_image
    ON {{TABLE_PREFIX}}image_generation_jobs (source_image_task_id, source_image_index)
    WHERE source_image_task_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS {{TABLE_PREFIX}}idx_image_generation_jobs_balance_idempotency_key
    ON {{TABLE_PREFIX}}image_generation_jobs (balance_idempotency_key)
    WHERE balance_idempotency_key IS NOT NULL;

CREATE TABLE IF NOT EXISTS {{TABLE_PREFIX}}image_public_gallery_items (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    source_task_id BIGINT NOT NULL,
    source_image_index INTEGER NOT NULL,
    image_url TEXT NOT NULL,
    prompt TEXT NOT NULL,
    is_visible BOOLEAN NOT NULL DEFAULT TRUE,
    created_from_public_generation BOOLEAN NOT NULL DEFAULT FALSE,
    published_at TIMESTAMPTZ NOT NULL,
    hidden_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT {{TABLE_PREFIX}}image_public_gallery_items_source_unique
        UNIQUE (source_task_id, source_image_index),
    CONSTRAINT {{TABLE_PREFIX}}image_public_gallery_items_source_image_index_check
        CHECK (source_image_index >= 0)
);

CREATE INDEX IF NOT EXISTS {{TABLE_PREFIX}}idx_image_public_gallery_visible_published
    ON {{TABLE_PREFIX}}image_public_gallery_items (is_visible, published_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS {{TABLE_PREFIX}}idx_image_public_gallery_user_visible
    ON {{TABLE_PREFIX}}image_public_gallery_items (user_id, is_visible, published_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS {{TABLE_PREFIX}}idx_image_public_gallery_source
    ON {{TABLE_PREFIX}}image_public_gallery_items (source_task_id, source_image_index);
