-- custom activity center schema.
-- This migration stays outside backend/migrations/*.sql so custom activity tables do not
-- consume upstream migration numbers or ent schema slots.

CREATE TABLE IF NOT EXISTS {{TABLE_PREFIX}}custom_activities (
    id BIGSERIAL PRIMARY KEY,
    type TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    cover_url TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    created_by BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at TIMESTAMPTZ,
    CONSTRAINT {{TABLE_PREFIX}}custom_activities_type_check
        CHECK (type IN ('red_packet_rain')),
    CONSTRAINT {{TABLE_PREFIX}}custom_activities_status_check
        CHECK (status IN ('draft', 'scheduled', 'active', 'ended', 'offline')),
    CONSTRAINT {{TABLE_PREFIX}}custom_activities_time_window_check
        CHECK (ends_at > starts_at)
);

CREATE INDEX IF NOT EXISTS {{TABLE_PREFIX}}idx_custom_activities_status_time
    ON {{TABLE_PREFIX}}custom_activities (status, starts_at, ends_at, id);

CREATE INDEX IF NOT EXISTS {{TABLE_PREFIX}}idx_custom_activities_type_status
    ON {{TABLE_PREFIX}}custom_activities (type, status, id);

CREATE TABLE IF NOT EXISTS {{TABLE_PREFIX}}custom_red_packet_rain_configs (
    activity_id BIGINT PRIMARY KEY
        REFERENCES {{TABLE_PREFIX}}custom_activities(id) ON DELETE CASCADE,
    round_count INTEGER NOT NULL CHECK (round_count > 0),
    round_duration_seconds INTEGER NOT NULL CHECK (round_duration_seconds > 0),
    round_interval_seconds INTEGER NOT NULL CHECK (round_interval_seconds >= 0),
    total_budget DECIMAL(18,8) NOT NULL CHECK (total_budget > 0),
    per_user_round_cap DECIMAL(18,8) NOT NULL CHECK (per_user_round_cap > 0),
    per_user_total_cap DECIMAL(18,8) NOT NULL CHECK (per_user_total_cap > 0),
    base_unit_amount DECIMAL(18,8) NOT NULL CHECK (base_unit_amount > 0),
    max_single_reward DECIMAL(18,8) NOT NULL CHECK (max_single_reward > 0),
    probability_step DECIMAL(10,8) NOT NULL CHECK (probability_step > 0 AND probability_step <= 1),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT {{TABLE_PREFIX}}custom_red_packet_rain_configs_user_total_budget_check
        CHECK (per_user_total_cap <= total_budget),
    CONSTRAINT {{TABLE_PREFIX}}custom_red_packet_rain_configs_single_reward_round_cap_check
        CHECK (max_single_reward <= per_user_round_cap)
);

CREATE TABLE IF NOT EXISTS {{TABLE_PREFIX}}custom_red_packet_rain_rounds (
    id BIGSERIAL PRIMARY KEY,
    activity_id BIGINT NOT NULL
        REFERENCES {{TABLE_PREFIX}}custom_activities(id) ON DELETE CASCADE,
    round_no INTEGER NOT NULL CHECK (round_no > 0),
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT {{TABLE_PREFIX}}custom_red_packet_rain_rounds_status_check
        CHECK (status IN ('waiting', 'active', 'ended', 'finished', 'offline')),
    CONSTRAINT {{TABLE_PREFIX}}custom_red_packet_rain_rounds_time_window_check
        CHECK (ends_at > starts_at),
    CONSTRAINT {{TABLE_PREFIX}}custom_red_packet_rain_rounds_activity_round_unique
        UNIQUE (activity_id, round_no)
);

CREATE INDEX IF NOT EXISTS {{TABLE_PREFIX}}idx_custom_red_packet_rain_rounds_activity_time
    ON {{TABLE_PREFIX}}custom_red_packet_rain_rounds (activity_id, starts_at, ends_at, round_no);

CREATE INDEX IF NOT EXISTS {{TABLE_PREFIX}}idx_custom_red_packet_rain_rounds_status_time
    ON {{TABLE_PREFIX}}custom_red_packet_rain_rounds (status, starts_at, ends_at, id);

CREATE TABLE IF NOT EXISTS {{TABLE_PREFIX}}custom_red_packet_rain_claims (
    id BIGSERIAL PRIMARY KEY,
    activity_id BIGINT NOT NULL
        REFERENCES {{TABLE_PREFIX}}custom_activities(id) ON DELETE CASCADE,
    round_id BIGINT NOT NULL
        REFERENCES {{TABLE_PREFIX}}custom_red_packet_rain_rounds(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL,
    hit_count INTEGER NOT NULL CHECK (hit_count >= 0),
    reward_amount DECIMAL(18,8) NOT NULL CHECK (reward_amount >= 0),
    idempotency_key TEXT NOT NULL CHECK (LENGTH(BTRIM(idempotency_key)) > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT {{TABLE_PREFIX}}custom_red_packet_rain_claims_zero_hit_reward_check
        CHECK (hit_count > 0 OR reward_amount = 0),
    CONSTRAINT {{TABLE_PREFIX}}custom_red_packet_rain_claims_idempotency_unique
        UNIQUE (activity_id, round_id, user_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS {{TABLE_PREFIX}}idx_custom_red_packet_rain_claims_activity_user
    ON {{TABLE_PREFIX}}custom_red_packet_rain_claims (activity_id, user_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS {{TABLE_PREFIX}}idx_custom_red_packet_rain_claims_round_user
    ON {{TABLE_PREFIX}}custom_red_packet_rain_claims (round_id, user_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS {{TABLE_PREFIX}}idx_custom_red_packet_rain_claims_activity_created
    ON {{TABLE_PREFIX}}custom_red_packet_rain_claims (activity_id, created_at DESC, id DESC);
