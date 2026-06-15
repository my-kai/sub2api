-- Persist OpenAI Image API output options carried by the frontend request.
-- Existing jobs keep NULL values and continue to use upstream defaults.

ALTER TABLE {{TABLE_PREFIX}}image_generation_jobs
    ADD COLUMN IF NOT EXISTS output_format TEXT,
    ADD COLUMN IF NOT EXISTS output_compression INTEGER;

ALTER TABLE {{TABLE_PREFIX}}image_generation_jobs
    ADD CONSTRAINT {{TABLE_PREFIX}}image_generation_jobs_output_compression_check
        CHECK (output_compression IS NULL OR (output_compression >= 0 AND output_compression <= 100))
        NOT VALID;
