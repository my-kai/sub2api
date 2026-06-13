-- Track whether an image generation session title was explicitly chosen by the user.
-- Auto prompt-based naming only applies to default titles that have not been customized.

ALTER TABLE {{TABLE_PREFIX}}image_generation_sessions
    ADD COLUMN IF NOT EXISTS title_customized BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE {{TABLE_PREFIX}}image_generation_sessions
SET title_customized = TRUE
WHERE title_customized = FALSE
  AND BTRIM(title) !~ '^新会话( [0-9]+)?$';
