-- Convert the already-applied multi-rule baseline to the single-rule schema.
-- The 001 file remains byte-for-byte immutable because production records its checksum.

ALTER TABLE custom_prompt_audit_v2_configs
    ADD COLUMN IF NOT EXISTS confidence_threshold DOUBLE PRECISION NOT NULL DEFAULT 0.8
        CHECK (confidence_threshold BETWEEN 0 AND 1),
    ADD COLUMN IF NOT EXISTS window_minutes INTEGER NOT NULL DEFAULT 60
        CHECK (window_minutes BETWEEN 1 AND 10080),
    ADD COLUMN IF NOT EXISTS trigger_count INTEGER NOT NULL DEFAULT 1
        CHECK (trigger_count BETWEEN 1 AND 10000),
    ADD COLUMN IF NOT EXISTS action VARCHAR(16) NOT NULL DEFAULT 'warning'
        CHECK (action IN ('ban', 'warning')),
    ADD COLUMN IF NOT EXISTS restriction_minutes INTEGER DEFAULT 30;

ALTER TABLE custom_prompt_audit_v2_configs
    ADD CONSTRAINT custom_prompt_audit_v2_configs_action_fields CHECK (
        (action = 'ban' AND restriction_minutes IS NULL) OR
        (action = 'warning' AND restriction_minutes BETWEEN 1 AND 43200)
    );

DO $$
DECLARE
    rule_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO rule_count
    FROM custom_prompt_audit_v2_rules;

    IF rule_count > 1 THEN
        RAISE EXCEPTION
            'cannot collapse % prompt audit v2 rules into one rule; remove duplicates explicitly',
            rule_count;
    ELSIF rule_count = 1 THEN
        UPDATE custom_prompt_audit_v2_configs AS config
        SET confidence_threshold = rule.confidence_threshold,
            window_minutes = rule.window_minutes,
            trigger_count = rule.trigger_count,
            action = rule.action,
            restriction_minutes = rule.restriction_minutes
        FROM (
            SELECT confidence_threshold, window_minutes, trigger_count,
                   action, restriction_minutes
            FROM custom_prompt_audit_v2_rules
            ORDER BY confidence_threshold DESC, config_order ASC, id ASC
            LIMIT 1
        ) AS rule
        WHERE config.id = 1;
    END IF;
END
$$;

DROP INDEX IF EXISTS idx_custom_prompt_audit_v2_rules_threshold;
DROP INDEX IF EXISTS idx_custom_prompt_audit_v2_events_user_rule_time;
DROP INDEX IF EXISTS idx_custom_prompt_audit_v2_events_rule_time;

ALTER TABLE custom_prompt_audit_v2_events
    DROP COLUMN IF EXISTS rule_id,
    DROP COLUMN IF EXISTS rule_name_snapshot;

ALTER TABLE custom_prompt_audit_v2_restrictions
    DROP COLUMN IF EXISTS rule_id;

DROP TABLE IF EXISTS custom_prompt_audit_v2_rules;
