-- +goose Up
-- briefs are weekly summaries for a user's party. input is the coach.Input
-- built when the brief is requested, so a remote worker only has to run the
-- coach on it.
CREATE TABLE briefs (
    id           bigserial PRIMARY KEY,
    steam_id     bigint      NOT NULL REFERENCES users (steam_id) ON DELETE CASCADE,
    period_start timestamptz NOT NULL,
    period_end   timestamptz NOT NULL,
    party        bigint[]    NOT NULL,
    status       text        NOT NULL DEFAULT 'queued', -- queued | ready | failed
    error        text,
    input        jsonb       NOT NULL,
    advice       jsonb,
    coach        text,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX briefs_user ON briefs (steam_id, created_at);

ALTER TABLE jobs ALTER COLUMN match_id DROP NOT NULL;
ALTER TABLE jobs ADD COLUMN brief_id bigint REFERENCES briefs (id) ON DELETE CASCADE;

-- +goose Down
ALTER TABLE jobs DROP COLUMN brief_id;
DELETE FROM jobs WHERE match_id IS NULL;
ALTER TABLE jobs ALTER COLUMN match_id SET NOT NULL;
DROP TABLE briefs;
