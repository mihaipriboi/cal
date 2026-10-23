-- A task may have no date (an inbox / "someday" item). Only plain tasks can be
-- undated: a time, repeat or reminder all need a day to anchor to.
-- +goose Up
ALTER TABLE entries ALTER COLUMN date DROP NOT NULL;
ALTER TABLE entries ADD CONSTRAINT entries_undated_check CHECK (
  date IS NOT NULL OR (type = 'task' AND start_time IS NULL AND end_time IS NULL
                       AND recur = 'none' AND remind IS NULL));
ALTER TABLE entry_revisions ALTER COLUMN date DROP NOT NULL;
-- Both default to false so a client that doesn't send them keeps the defaults:
-- quick-added tasks without a date stay undated, and the No date group is last.
ALTER TABLE settings ADD COLUMN quick_add_today BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE settings ADD COLUMN undated_first BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE settings DROP COLUMN undated_first;
ALTER TABLE settings DROP COLUMN quick_add_today;
-- Undated tasks get their creation day; revisions without a date are dropped.
DELETE FROM entry_revisions WHERE date IS NULL;
UPDATE entries SET date = created_at::date WHERE date IS NULL;
ALTER TABLE entries DROP CONSTRAINT entries_undated_check;
ALTER TABLE entries ALTER COLUMN date SET NOT NULL;
ALTER TABLE entry_revisions ALTER COLUMN date SET NOT NULL;
