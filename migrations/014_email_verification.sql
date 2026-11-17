-- Existing users and sessions remain usable; no invented verification dates.
ALTER TABLE users ADD COLUMN email_verified_at TIMESTAMPTZ NULL;
ALTER TABLE users ADD COLUMN email_verification_required BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE users ADD CONSTRAINT users_email_verified_time CHECK(email_verified_at IS NULL OR (email_verified_at>=created_at AND email_verified_at<=updated_at));

CREATE TABLE email_verification_tokens (
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
CREATE UNIQUE INDEX email_verification_one_live ON email_verification_tokens(user_id) WHERE used_at IS NULL AND revoked_at IS NULL;
CREATE INDEX email_verification_user_recent ON email_verification_tokens(user_id,created_at DESC);
CREATE INDEX email_verification_expiry ON email_verification_tokens(expires_at);

-- Both successful and unknown-account resend attempts consume IP quota.
-- Only purpose-separated HMACs of trusted ClientIP are stored, never raw IPs.
CREATE TABLE email_verification_ip_limits (
 ip_hash BYTEA PRIMARY KEY CHECK(octet_length(ip_hash)=32),
 minute_started_at TIMESTAMPTZ NOT NULL,
 hour_started_at TIMESTAMPTZ NOT NULL,
 minute_count INTEGER NOT NULL CHECK(minute_count>=0),
 hour_count INTEGER NOT NULL CHECK(hour_count>=0)
);
