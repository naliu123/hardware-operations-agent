CREATE UNIQUE INDEX IF NOT EXISTS response_request_key
ON responses ((payload->>'conversation_id'), (payload->>'request_key'))
WHERE COALESCE(payload->>'request_key','') <> '';

CREATE TABLE IF NOT EXISTS response_events (
    response_id text NOT NULL REFERENCES responses(id),
    sequence bigint NOT NULL,
    payload jsonb NOT NULL,
    PRIMARY KEY (response_id, sequence)
);
