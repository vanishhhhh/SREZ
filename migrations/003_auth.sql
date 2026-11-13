-- Existing collisions must be resolved by an operator with ownership evidence.
-- No records are renamed, merged, deleted or assigned passwords here.
CREATE UNIQUE INDEX users_email_auth_unique ON users(lower(btrim(email)));
CREATE UNIQUE INDEX users_username_auth_unique ON users(lower(username));
CREATE TABLE auth_sessions (
 id TEXT PRIMARY KEY,
 user_id BIGINT NOT NULL REFERENCES users(id),
 created_at TIMESTAMPTZ NOT NULL,
 expires_at TIMESTAMPTZ NOT NULL CHECK(expires_at > created_at AND expires_at <= created_at + interval '7 days'),
 last_used_at TIMESTAMPTZ NOT NULL,
 revoked_at TIMESTAMPTZ
);
CREATE INDEX auth_sessions_user ON auth_sessions(user_id);
CREATE TABLE auth_refresh_tokens (
 token_hash BYTEA PRIMARY KEY CHECK(octet_length(token_hash)=32),
 session_id TEXT NOT NULL REFERENCES auth_sessions(id),
 used_at TIMESTAMPTZ
);
CREATE INDEX auth_refresh_session ON auth_refresh_tokens(session_id);
CREATE UNIQUE INDEX auth_refresh_current ON auth_refresh_tokens(session_id) WHERE used_at IS NULL;
