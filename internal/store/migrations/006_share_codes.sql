-- +goose Up
-- steam_links hold what the share-code importer needs to read a user's
-- match history from the Steam Web API: the match-history auth code the
-- player creates in Steam's help pages and the newest share code seen so far.
CREATE TABLE steam_links (
    steam_id   bigint      PRIMARY KEY REFERENCES users (steam_id) ON DELETE CASCADE,
    auth_code  text        NOT NULL,
    last_code  text        NOT NULL,
    checked_at timestamptz,
    -- error is set when Steam rejects the auth code or the share code; the
    -- importer skips the link until the player saves it again.
    error      text        NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

-- share_codes are the matches the importer found. They wait for a Game
-- Coordinator client to turn them into demo URLs (ADR 9).
CREATE TABLE share_codes (
    id         bigserial PRIMARY KEY,
    steam_id   bigint      NOT NULL REFERENCES users (steam_id) ON DELETE CASCADE,
    code       text        NOT NULL,
    status     text        NOT NULL DEFAULT 'waiting',
    match_id   bigint      REFERENCES matches (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (steam_id, code)
);

-- +goose Down
DROP TABLE share_codes;
DROP TABLE steam_links;
