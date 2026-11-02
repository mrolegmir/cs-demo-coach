-- +goose Up
-- practice is a player's own log of what they trained, optionally tied to one
-- kind of mistake, so progress pages and briefs can tell whether it helps.
CREATE TABLE practice (
    id           bigserial PRIMARY KEY,
    steam_id     bigint      NOT NULL REFERENCES users (steam_id) ON DELETE CASCADE,
    kind         text        NOT NULL DEFAULT '', -- detect.Kind, or '' for general practice
    note         text        NOT NULL,
    minutes      int         NOT NULL DEFAULT 0,
    practiced_on date        NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX practice_user ON practice (steam_id, practiced_on);

-- +goose Down
DROP TABLE practice;
