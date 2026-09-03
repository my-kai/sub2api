CREATE TABLE IF NOT EXISTS {{TABLE_PREFIX}}group_model_rate_multipliers (
    id BIGSERIAL PRIMARY KEY,
    group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    model TEXT NOT NULL,
    rate_multiplier DOUBLE PRECISION NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT {{TABLE_PREFIX}}group_model_rate_multipliers_model_not_empty CHECK (btrim(model) <> ''),
    CONSTRAINT {{TABLE_PREFIX}}group_model_rate_multipliers_rate_valid CHECK (
        rate_multiplier >= 0 AND rate_multiplier < 'Infinity'::double precision
    ),
    CONSTRAINT {{TABLE_PREFIX}}group_model_rate_multipliers_group_model_key UNIQUE (group_id, model)
);

CREATE INDEX IF NOT EXISTS {{TABLE_PREFIX}}group_model_rate_multipliers_group_id_idx
    ON {{TABLE_PREFIX}}group_model_rate_multipliers (group_id);
