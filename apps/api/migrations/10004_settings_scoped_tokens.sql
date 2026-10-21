-- One token per capability: api_token stays for MCP (full access), the email
-- intake gets a create-only intake_token, and the ICS export a feed_token
-- instead of sharing the today-widget token. Each new token starts as a copy
-- of the token it replaces, so existing URLs keep working until rotated.
-- +goose Up
ALTER TABLE settings
  ADD COLUMN intake_token TEXT NOT NULL DEFAULT replace(gen_random_uuid()::text, '-', ''),
  ADD COLUMN feed_token   TEXT NOT NULL DEFAULT replace(gen_random_uuid()::text, '-', '');
UPDATE settings SET intake_token = api_token, feed_token = widget_token;

-- +goose Down
ALTER TABLE settings DROP COLUMN intake_token, DROP COLUMN feed_token;
