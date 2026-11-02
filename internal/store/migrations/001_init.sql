-- +goose Up
CREATE TABLE matches (
    id          bigserial PRIMARY KEY,
    filename    text        NOT NULL,
    uploaded_at timestamptz NOT NULL DEFAULT now(),
    -- uploaded -> detected -> coached; failed at any step.
    status      text        NOT NULL DEFAULT 'uploaded',
    error       text,
    map         text,
    rounds      int,
    -- party is the players chosen for coaching, as SteamID64s.
    party       bigint[],
    -- data is the parsed match.Match, report the detect.Report and advice
    -- the coach.Advice, all as JSON.
    data        jsonb,
    report      jsonb,
    advice      jsonb,
    coach       text
);

CREATE TABLE match_players (
    match_id  bigint NOT NULL REFERENCES matches (id) ON DELETE CASCADE,
    player_id bigint NOT NULL,
    name      text   NOT NULL,
    -- side in the first round the player played: 1 = T, 2 = CT.
    side      smallint NOT NULL,
    PRIMARY KEY (match_id, player_id)
);

-- findings are kept as rows so issue history across matches can be queried.
CREATE TABLE findings (
    match_id   bigint NOT NULL REFERENCES matches (id) ON DELETE CASCADE,
    finding_id text   NOT NULL,
    kind       text   NOT NULL,
    player_id  bigint NOT NULL,
    round      int    NOT NULL,
    tick       int    NOT NULL,
    detail     text   NOT NULL,
    PRIMARY KEY (match_id, finding_id)
);
CREATE INDEX findings_player_kind ON findings (player_id, kind);

-- jobs is the work queue; workers claim rows with FOR UPDATE SKIP LOCKED.
CREATE TABLE jobs (
    id         bigserial PRIMARY KEY,
    kind       text        NOT NULL, -- detect | coach
    match_id   bigint      NOT NULL REFERENCES matches (id) ON DELETE CASCADE,
    status     text        NOT NULL DEFAULT 'queued', -- queued | running | done | failed
    attempts   int         NOT NULL DEFAULT 0,
    run_after  timestamptz NOT NULL DEFAULT now(),
    locked_at  timestamptz,
    error      text,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX jobs_queued ON jobs (run_after) WHERE status = 'queued';

-- +goose Down
DROP TABLE jobs;
DROP TABLE findings;
DROP TABLE match_players;
DROP TABLE matches;
