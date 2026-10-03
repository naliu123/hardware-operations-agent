CREATE TABLE IF NOT EXISTS incidents (
    id text PRIMARY KEY,
    request_key text UNIQUE,
    payload jsonb NOT NULL
);
CREATE TABLE IF NOT EXISTS diagnostic_runs (
    id text PRIMARY KEY,
    incident_id text NOT NULL REFERENCES incidents(id),
    state_version bigint NOT NULL,
    status text NOT NULL,
    payload jsonb NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS diagnostic_runs_one_active
    ON diagnostic_runs(incident_id) WHERE status NOT IN ('COMPLETED', 'CANCELED');
CREATE INDEX IF NOT EXISTS diagnostic_runs_pending
    ON diagnostic_runs(status) WHERE status IN ('QUEUED', 'RUNNING');
