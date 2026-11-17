-- Additive and purpose-separated from email verification and refresh tokens.
CREATE TABLE password_reset_tokens (
 id BIGSERIAL PRIMARY KEY,
 user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 token_hash BYTEA NOT NULL UNIQUE CHECK(octet_length(token_hash)=32),
 created_at TIMESTAMPTZ NOT NULL,
 expires_at TIMESTAMPTZ NOT NULL,
 used_at TIMESTAMPTZ,
 revoked_at TIMESTAMPTZ,
 CHECK(expires_at=created_at+interval '30 minutes'),
 CHECK(used_at IS NULL OR used_at>=created_at),
 CHECK(revoked_at IS NULL OR revoked_at>=created_at),
 CHECK(used_at IS NULL OR revoked_at IS NULL)
);
CREATE UNIQUE INDEX password_reset_one_live ON password_reset_tokens(user_id) WHERE used_at IS NULL AND revoked_at IS NULL;
CREATE INDEX password_reset_user_recent ON password_reset_tokens(user_id,created_at DESC);
CREATE INDEX password_reset_expiry ON password_reset_tokens(expires_at);
CREATE TABLE password_reset_ip_limits (
 ip_hash BYTEA PRIMARY KEY CHECK(octet_length(ip_hash)=32),
 minute_started_at TIMESTAMPTZ NOT NULL,
 hour_started_at TIMESTAMPTZ NOT NULL,
 minute_count INTEGER NOT NULL CHECK(minute_count>=0),
 hour_count INTEGER NOT NULL CHECK(hour_count>=0)
);
