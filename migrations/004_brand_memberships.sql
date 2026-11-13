CREATE TABLE brand_memberships (
 user_id BIGINT NOT NULL REFERENCES users(id),
 brand_id BIGINT NOT NULL REFERENCES brands(id),
 role TEXT NOT NULL CHECK(role IN ('owner','editor')),
 status TEXT NOT NULL CHECK(status IN ('invited','active','revoked','declined')),
 invited_by_user_id BIGINT REFERENCES users(id),
 invited_at TIMESTAMPTZ,
 joined_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL,
 updated_at TIMESTAMPTZ NOT NULL CHECK(updated_at >= created_at),
 PRIMARY KEY(user_id,brand_id),
 CHECK((invited_at IS NULL) = (invited_by_user_id IS NULL)),
 CHECK(invited_at IS NULL OR invited_at BETWEEN created_at AND updated_at),
 CHECK(joined_at IS NULL OR joined_at BETWEEN created_at AND updated_at),
 CHECK(invited_at IS NULL OR joined_at IS NULL OR joined_at >= invited_at),
 CHECK(role <> 'owner' OR (status='active' AND joined_at IS NOT NULL)),
 CHECK(role <> 'editor' OR
   (status IN ('invited','declined') AND invited_at IS NOT NULL AND joined_at IS NULL) OR
   (status='active' AND joined_at IS NOT NULL) OR
   (status='revoked' AND (invited_at IS NOT NULL OR joined_at IS NOT NULL)))
);
CREATE INDEX brand_memberships_brand ON brand_memberships(brand_id,user_id);
CREATE UNIQUE INDEX brand_memberships_owner ON brand_memberships(brand_id) WHERE role='owner' AND status='active';
-- Legacy brands are intentionally not assigned owners. No ownership transfer API.
