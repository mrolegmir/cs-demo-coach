-- +goose Up
-- telegram_links is the chat a user's briefs are sent to (ADR 13).
CREATE TABLE telegram_links (
    steam_id   bigint      PRIMARY KEY REFERENCES users (steam_id) ON DELETE CASCADE,
    chat_id    bigint      NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- telegram_codes are one-time codes that link a chat: the site makes one and
-- opens the bot with it, the bot gets it back with the chat. Only a SHA-256
-- of the code is stored, as for upload tokens.
CREATE TABLE telegram_codes (
    code_hash  bytea       PRIMARY KEY,
    steam_id   bigint      NOT NULL REFERENCES users (steam_id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL
);

-- ready_at is when the coach finished a brief (a home worker may take days
-- after it was queued); notified_at is when it was sent to Telegram.
ALTER TABLE briefs ADD COLUMN ready_at timestamptz;
ALTER TABLE briefs ADD COLUMN notified_at timestamptz;

-- +goose Down
ALTER TABLE briefs DROP COLUMN notified_at;
ALTER TABLE briefs DROP COLUMN ready_at;
DROP TABLE telegram_codes;
DROP TABLE telegram_links;
