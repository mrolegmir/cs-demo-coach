-- +goose Up
-- users are Steam accounts allowed in; the SteamID64 is the key, so a user's
-- matches and findings join on player_id without any mapping.
CREATE TABLE users (
    steam_id   bigint PRIMARY KEY,
    admin      boolean     NOT NULL DEFAULT false,
    invited_by bigint REFERENCES users (steam_id),
    created_at timestamptz NOT NULL DEFAULT now()
);

-- sessions stores only a SHA-256 of the cookie value.
CREATE TABLE sessions (
    token_hash bytea PRIMARY KEY,
    steam_id   bigint      NOT NULL REFERENCES users (steam_id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL
);
CREATE INDEX sessions_expires ON sessions (expires_at);

CREATE TABLE invites (
    code       text PRIMARY KEY,
    created_by bigint      NOT NULL REFERENCES users (steam_id),
    used_by    bigint REFERENCES users (steam_id),
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE matches ADD COLUMN uploaded_by bigint REFERENCES users (steam_id);
CREATE INDEX matches_uploaded_by ON matches (uploaded_by, uploaded_at);
CREATE INDEX match_players_player ON match_players (player_id);
ALTER TABLE jobs ADD COLUMN requested_by bigint REFERENCES users (steam_id);
CREATE INDEX jobs_requested_by ON jobs (requested_by, created_at);

-- +goose Down
DROP INDEX jobs_requested_by;
ALTER TABLE jobs DROP COLUMN requested_by;
DROP INDEX match_players_player;
DROP INDEX matches_uploaded_by;
ALTER TABLE matches DROP COLUMN uploaded_by;
DROP TABLE invites;
DROP TABLE sessions;
DROP TABLE users;
