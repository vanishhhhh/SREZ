CREATE TABLE brand_pages (
    brand_id BIGINT PRIMARY KEY REFERENCES brands(id) ON DELETE CASCADE,
    revision BIGINT NOT NULL CHECK (revision BETWEEN 1 AND 9007199254740991),
    draft JSONB NOT NULL CHECK (jsonb_typeof(draft) = 'object' AND octet_length(draft::text) <= 8192),
    published JSONB CHECK (jsonb_typeof(published) = 'object' AND octet_length(published::text) <= 8192),
    published_revision BIGINT CHECK (published_revision BETWEEN 1 AND revision),
    CHECK ((published IS NULL) = (published_revision IS NULL))
);
