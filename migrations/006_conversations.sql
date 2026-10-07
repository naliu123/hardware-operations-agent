ALTER TABLE conversations ADD COLUMN deleted_at timestamptz;
UPDATE conversations SET payload = payload || jsonb_build_object(
    'title','新会话','title_source','AUTO','state_version',1,
    'last_sequence',0,'updated_at',payload->'created_at');
WITH ordered AS (
    SELECT id, row_number() OVER (
        PARTITION BY payload->>'conversation_id' ORDER BY payload->>'created_at',id) AS seq
    FROM responses
)
UPDATE responses r SET payload=r.payload || jsonb_build_object('sequence',o.seq)
FROM ordered o WHERE r.id=o.id;
UPDATE conversations c SET payload=jsonb_set(c.payload,'{last_sequence}',to_jsonb((
    SELECT COALESCE(MAX((r.payload->>'sequence')::bigint),0)
    FROM responses r WHERE r.payload->>'conversation_id'=c.id)));
CREATE UNIQUE INDEX response_conversation_sequence
    ON responses ((payload->>'conversation_id'),((payload->>'sequence')::bigint));
CREATE UNIQUE INDEX response_active_workbench
    ON responses ((payload->>'conversation_id'))
    WHERE payload->>'workbench'='true' AND payload->>'status' IN ('QUEUED','RUNNING','CANCELING');
CREATE INDEX conversations_visible ON conversations(owner_id,id) WHERE deleted_at IS NULL;
CREATE TABLE conversation_summaries (
    conversation_id text NOT NULL REFERENCES conversations(id),
    version bigint NOT NULL,
    payload jsonb NOT NULL,
    PRIMARY KEY(conversation_id,version)
);
CREATE TABLE cleanup_jobs (
    conversation_id text PRIMARY KEY REFERENCES conversations(id),
    status text NOT NULL DEFAULT 'PENDING',
    attempts integer NOT NULL DEFAULT 0,
    last_error text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now()
);
