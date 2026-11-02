-- +goose Up
ALTER TABLE users ADD COLUMN auto_coaching boolean NOT NULL DEFAULT false;
ALTER TABLE matches ADD COLUMN party_manual boolean NOT NULL DEFAULT false;
CREATE TABLE party_friends (
    owner_id bigint NOT NULL REFERENCES users (steam_id) ON DELETE CASCADE,
    friend_id bigint NOT NULL CHECK (friend_id > 0),
    PRIMARY KEY (owner_id, friend_id),
    CHECK (owner_id <> friend_id)
);

-- +goose Down
DROP TABLE party_friends;
ALTER TABLE users DROP COLUMN auto_coaching;
ALTER TABLE matches DROP COLUMN party_manual;
