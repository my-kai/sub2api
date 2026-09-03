-- Store the configured proxy host used by each request; NULL preserves history.
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS egress_host TEXT;
