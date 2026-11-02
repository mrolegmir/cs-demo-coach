-- +goose Up
-- Snapshot the browser's language on the queued request: the worker has no
-- browser session, and retries must produce advice in the same language.
ALTER TABLE jobs ADD COLUMN language text NOT NULL DEFAULT 'ru'
    CHECK (language IN ('ru', 'en'));

-- +goose Down
ALTER TABLE jobs DROP COLUMN language;
