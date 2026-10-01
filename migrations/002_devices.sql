-- +goose Up
CREATE TABLE IF NOT EXISTS device_snapshots (
    id text PRIMARY KEY,
    observed_at timestamptz NOT NULL,
    payload jsonb NOT NULL,
    CHECK (payload->>'device_id' = id)
);

CREATE TABLE IF NOT EXISTS version_policies (
    id text PRIMARY KEY,
    payload jsonb NOT NULL,
    CHECK (payload->>'id' = id)
);
