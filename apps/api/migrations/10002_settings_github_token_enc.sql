-- The GitHub token is stored encrypted, like other credentials. The key lives
-- outside the database, so SQL can't encrypt: the server moves existing
-- plaintext values into github_token_enc on start (store.InitSecrets).
-- +goose Up
ALTER TABLE settings ADD COLUMN github_token_enc TEXT;

-- +goose Down
-- Encrypted tokens can't be decrypted here; enter the token again after a
-- downgrade.
ALTER TABLE settings DROP COLUMN github_token_enc;
