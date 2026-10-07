CREATE TABLE attachments (
    id text PRIMARY KEY,
    owner_id text NOT NULL REFERENCES users(id),
    conversation_id text NOT NULL REFERENCES conversations(id),
    storage_key text NOT NULL UNIQUE,
    status text NOT NULL,
    version bigint NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    payload jsonb NOT NULL,
    CHECK (payload->>'id' = id),
    CHECK (status IN ('UPLOADED','PROCESSING','READY','PARTIAL','FAILED','UNSUPPORTED'))
);
CREATE INDEX attachments_conversation ON attachments(conversation_id,created_at,id);

CREATE TABLE attachment_parse_jobs (
    attachment_id text PRIMARY KEY REFERENCES attachments(id),
    status text NOT NULL,
    version bigint NOT NULL,
    deadline timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CHECK (status IN ('QUEUED','PROCESSING','DONE'))
);
CREATE INDEX attachment_parse_pending ON attachment_parse_jobs(status,updated_at);

CREATE TABLE attachment_pages (
    attachment_id text NOT NULL REFERENCES attachments(id),
    page_number integer NOT NULL CHECK (page_number > 0),
    status text NOT NULL,
    payload jsonb NOT NULL,
    PRIMARY KEY(attachment_id,page_number)
);

CREATE TABLE attachment_assets (
    id text PRIMARY KEY,
    attachment_id text NOT NULL REFERENCES attachments(id),
    page_number integer NOT NULL CHECK (page_number > 0),
    storage_key text NOT NULL UNIQUE,
    payload jsonb NOT NULL,
    CHECK (payload->>'id' = id)
);
CREATE INDEX attachment_assets_page ON attachment_assets(attachment_id,page_number);

CREATE TABLE message_attachments (
    response_id text NOT NULL REFERENCES responses(id),
    attachment_id text NOT NULL REFERENCES attachments(id),
    ordinal integer NOT NULL CHECK (ordinal >= 0 AND ordinal < 5),
    PRIMARY KEY(response_id,attachment_id),
    UNIQUE(response_id,ordinal)
);
