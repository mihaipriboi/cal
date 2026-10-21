-- Closing the linked GitHub issue when a board card is completed writes to
-- GitHub with the user's token; it is now an explicit setting, off by default.
-- +goose Up
ALTER TABLE settings ADD COLUMN github_close_issues BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE settings DROP COLUMN github_close_issues;
