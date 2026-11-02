-- +goose Up
-- The bot resolves a waiting share code to a demo URL through the Game
-- Coordinator; the api then downloads the demo and imports it like an upload.
-- status: waiting → resolving (bot) → download → downloading (api) →
-- imported, or failed / expired / over_quota.
ALTER TABLE share_codes
    ADD COLUMN attempts   int         NOT NULL DEFAULT 0,
    ADD COLUMN claimed_at timestamptz,
    ADD COLUMN demo_url   text        NOT NULL DEFAULT '',
    ADD COLUMN played_at  timestamptz,
    ADD COLUMN reason     text        NOT NULL DEFAULT '';
CREATE INDEX share_codes_status ON share_codes (status, id);

-- +goose Down
DROP INDEX share_codes_status;
ALTER TABLE share_codes
    DROP COLUMN reason,
    DROP COLUMN played_at,
    DROP COLUMN demo_url,
    DROP COLUMN claimed_at,
    DROP COLUMN attempts;
