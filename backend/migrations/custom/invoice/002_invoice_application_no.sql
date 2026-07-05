ALTER TABLE custom_invoice_applications
    ADD COLUMN IF NOT EXISTS application_no TEXT NOT NULL DEFAULT '';

-- Historical rows get deterministic unordered suffixes before the unique
-- constraint is added. The fixed salt keeps the value reproducible without
-- exposing date-local application counts.
WITH source AS (
    SELECT id,
           to_char(created_at AT TIME ZONE 'Asia/Shanghai', 'YYYYMMDD') AS application_date,
           md5('custom-invoice-application-no-v1:' || id::text || ':' || created_at::text) AS hash
    FROM custom_invoice_applications
    WHERE application_no = ''
       OR application_no !~ '^INV[0-9]{8}-[23456789ABCDEFGHJKLMNPQRSTUVWXYZ]{10}$'
),
suffixes AS (
    SELECT source.id,
           source.application_date,
           string_agg(
               substr(
                   '23456789ABCDEFGHJKLMNPQRSTUVWXYZ',
                   (get_byte(decode(substr(source.hash, seq.pos * 2 + 1, 2), 'hex'), 0) % 32) + 1,
                   1
               ),
               ''
               ORDER BY seq.pos
           ) AS suffix
    FROM source
    CROSS JOIN generate_series(0, 9) AS seq(pos)
    GROUP BY source.id, source.application_date
)
UPDATE custom_invoice_applications
SET application_no = 'INV' || suffixes.application_date || '-' || suffixes.suffix
FROM suffixes
WHERE custom_invoice_applications.id = suffixes.id;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'custom_invoice_applications_application_no_required'
          AND conrelid = 'custom_invoice_applications'::regclass
    ) THEN
        ALTER TABLE custom_invoice_applications
            ADD CONSTRAINT custom_invoice_applications_application_no_required
            CHECK (application_no <> '');
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS idx_custom_invoice_applications_application_no
    ON custom_invoice_applications(application_no);

ALTER TABLE custom_invoice_applications
    ALTER COLUMN application_no DROP DEFAULT;
