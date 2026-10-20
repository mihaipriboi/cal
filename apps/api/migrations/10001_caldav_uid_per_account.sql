-- CalDAV object UIDs are unique per calendar account, not globally: the same
-- UID can legitimately exist in two accounts (one event in two calendars), and
-- one account's sync must never update another account's rows.
-- +goose Up
DROP INDEX IF EXISTS entries_external_uid_key;
CREATE UNIQUE INDEX entries_account_external_uid_key
  ON entries (account_id, external_uid) WHERE external_uid IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS entries_account_external_uid_key;
-- Fails if the same UID now exists in two accounts; dedupe those rows first.
CREATE UNIQUE INDEX entries_external_uid_key ON entries (external_uid) WHERE external_uid IS NOT NULL;
