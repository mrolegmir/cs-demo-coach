-- +goose Up
-- A durable cleanup marker: deleting the file is retried after result commit.
ALTER TABLE jobs ADD COLUMN demo_deleted boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE jobs DROP COLUMN demo_deleted;
