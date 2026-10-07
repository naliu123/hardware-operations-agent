CREATE TABLE users (
    id text PRIMARY KEY,
    username text NOT NULL UNIQUE CHECK (username = lower(username)),
    password_hash text NOT NULL,
    role text NOT NULL CHECK (role IN ('ADMIN','USER')),
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE auth_sessions (
    token_hash text PRIMARY KEY,
    user_id text NOT NULL REFERENCES users(id),
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX auth_sessions_user ON auth_sessions(user_id);
CREATE TABLE login_limits (
    key text PRIMARY KEY,
    attempts integer NOT NULL,
    expires_at timestamptz NOT NULL
);
ALTER TABLE conversations ADD COLUMN owner_id text REFERENCES users(id);
CREATE INDEX conversations_owner ON conversations(owner_id, id);
-- Old local-operator records deliberately remain unowned until explicit mapping.
CREATE TABLE ownership_mappings (
    source_owner text PRIMARY KEY,
    target_user_id text NOT NULL REFERENCES users(id),
    mapped_count bigint NOT NULL,
    mapped_at timestamptz NOT NULL DEFAULT now()
);
