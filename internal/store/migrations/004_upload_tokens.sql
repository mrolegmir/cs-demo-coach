-- +goose Up
-- upload_tokens let a user's own tools (csdc watch) upload demos. Like
-- sessions, only a SHA-256 of the token is stored.
CREATE TABLE upload_tokens (
    id           bigserial PRIMARY KEY,
    token_hash   bytea       NOT NULL UNIQUE,
    steam_id     bigint      NOT NULL REFERENCES users (steam_id) ON DELETE CASCADE,
    name         text        NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz
);
CREATE INDEX upload_tokens_user ON upload_tokens (steam_id);

-- sha256 of the demo file, so uploading the same demo twice (from the
-- watcher and the browser, say) returns the first match.
ALTER TABLE matches ADD COLUMN sha256 bytea;
CREATE UNIQUE INDEX matches_upload_sha256 ON matches (uploaded_by, sha256) WHERE sha256 IS NOT NULL;

-- played_at is when the match was played, if the uploader knows (the
-- watcher sends the demo file's time); history falls back to uploaded_at.
ALTER TABLE matches ADD COLUMN played_at timestamptz;

-- +goose Down
ALTER TABLE matches DROP COLUMN played_at;
DROP INDEX matches_upload_sha256;
ALTER TABLE matches DROP COLUMN sha256;
DROP TABLE upload_tokens;
