-- Independent of product_color_images: no public URL or publication in Phase 1.
-- Resource IDs are historical snapshots, authorized against live rows at each
-- operation. No FK may block existing color/product deletion or erase GC keys.
CREATE TABLE media_uploads (
 id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 user_id BIGINT NOT NULL REFERENCES users(id),
 brand_id BIGINT NOT NULL CHECK(brand_id > 0),
 product_id BIGINT NOT NULL CHECK(product_id > 0),
 color_id BIGINT NOT NULL CHECK(color_id > 0),
 bucket TEXT NOT NULL,
 staging_key TEXT NOT NULL UNIQUE CHECK(staging_key LIKE 'staging/%'),
 object_key TEXT NOT NULL UNIQUE CHECK(object_key LIKE 'ready/%'),
 expected_size BIGINT NOT NULL CHECK(expected_size BETWEEN 1 AND 5242880),
 actual_size BIGINT CHECK(actual_size > 0),
 mime TEXT NOT NULL CHECK(mime IN ('image/jpeg','image/png','image/webp')),
 status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','ready','rejected')),
 processing BOOLEAN NOT NULL DEFAULT false,
 cleaned BOOLEAN NOT NULL DEFAULT false,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 expires_at TIMESTAMPTZ NOT NULL DEFAULT now() + interval '15 minutes',
 confirmed_at TIMESTAMPTZ,
 next_cleanup_at TIMESTAMPTZ NOT NULL DEFAULT now() + interval '20 minutes',
 CHECK((status='ready') = (confirmed_at IS NOT NULL)),
 CHECK(status <> 'ready' OR actual_size IS NOT NULL)
);
CREATE INDEX media_uploads_cleanup ON media_uploads(next_cleanup_at);
CREATE INDEX media_uploads_user ON media_uploads(user_id,id);
