-- +goose Up
CREATE TABLE IF NOT EXISTS knowledge_revisions (
    id text PRIMARY KEY,
    payload jsonb NOT NULL,
    CHECK (payload->>'id' = id)
);
CREATE INDEX IF NOT EXISTS knowledge_publication_status
    ON knowledge_revisions ((payload->>'status'));
CREATE TABLE IF NOT EXISTS conversations (
    id text PRIMARY KEY,
    payload jsonb NOT NULL,
    CHECK (payload->>'id' = id)
);
CREATE TABLE IF NOT EXISTS responses (
    id text PRIMARY KEY,
    payload jsonb NOT NULL,
    CHECK (payload->>'id' = id)
);
CREATE INDEX IF NOT EXISTS response_status
    ON responses ((payload->>'status'));
