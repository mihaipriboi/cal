-- Sessions are stored as the SHA-256 (hex) of the token the client holds, so
-- the table and database backups contain nothing usable to sign in. Existing
-- rows are hashed in place, so current logins stay valid.
-- +goose Up
ALTER TABLE sessions ALTER COLUMN id DROP DEFAULT;
ALTER TABLE sessions ALTER COLUMN id TYPE TEXT USING encode(sha256(convert_to(id::text, 'UTF8')), 'hex');

-- +goose Down
-- Hashes can't be turned back into tokens: everyone signs in again.
DELETE FROM sessions;
ALTER TABLE sessions ALTER COLUMN id TYPE UUID USING gen_random_uuid();
ALTER TABLE sessions ALTER COLUMN id SET DEFAULT gen_random_uuid();
