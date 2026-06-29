-- Tighten gift-credit grant source identity without editing the already-applied 001 migration.
-- Existing rows are backfilled with their id so the new NOT NULL / non-blank contract is explicit.

UPDATE {{TABLE_PREFIX}}custom_gift_credit_grants
SET source_id = 'legacy:' || id::text
WHERE btrim(COALESCE(source_id, '')) = '';

ALTER TABLE {{TABLE_PREFIX}}custom_gift_credit_grants
    ALTER COLUMN source_id DROP DEFAULT;

ALTER TABLE {{TABLE_PREFIX}}custom_gift_credit_grants
    ALTER COLUMN source_id SET NOT NULL;

ALTER TABLE {{TABLE_PREFIX}}custom_gift_credit_grants
    DROP CONSTRAINT IF EXISTS {{TABLE_PREFIX}}custom_gift_credit_grants_source_id_required;

ALTER TABLE {{TABLE_PREFIX}}custom_gift_credit_grants
    ADD CONSTRAINT {{TABLE_PREFIX}}custom_gift_credit_grants_source_id_required
        CHECK (btrim(source_id) <> '');
