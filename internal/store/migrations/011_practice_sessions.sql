-- +goose Up
-- Starts are separate from completed practice, including for older workers.
CREATE TABLE practice_sessions (
    id bigserial PRIMARY KEY,
    steam_id bigint NOT NULL REFERENCES users (steam_id) ON DELETE CASCADE,
    drill text NOT NULL,
    minutes int NOT NULL CHECK (minutes BETWEEN 0 AND 600),
    started_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (steam_id, drill)
);

-- +goose Down
DROP TABLE practice_sessions;
