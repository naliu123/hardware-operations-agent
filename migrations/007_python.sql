CREATE TABLE python_executions (
    id text PRIMARY KEY,
    owner_id text NOT NULL REFERENCES users(id),
    conversation_id text NOT NULL REFERENCES conversations(id),
    response_id text NOT NULL REFERENCES responses(id),
    status text NOT NULL,
    version bigint NOT NULL,
    created_at timestamptz NOT NULL,
    payload jsonb NOT NULL
);
CREATE INDEX python_pending ON python_executions(status,created_at);
CREATE INDEX python_response ON python_executions(response_id);
CREATE TABLE artifacts (
    id text PRIMARY KEY,
    execution_id text NOT NULL REFERENCES python_executions(id),
    storage_key text NOT NULL UNIQUE,
    payload jsonb NOT NULL
);
