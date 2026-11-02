-- +goose Up
-- drill is the library drill (internal/drills) an entry was for, or '' for
-- the player's own practice.
ALTER TABLE practice ADD COLUMN drill text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE practice DROP COLUMN drill;
