-- Phase 1 journal is retained. Cancel its unvalidated uploads; retain old ready
-- private objects for seven days before reconciliation. No old URL can publish.
ALTER TABLE media_uploads ADD COLUMN retired_at TIMESTAMPTZ NOT NULL DEFAULT now();
UPDATE media_uploads SET status='rejected' WHERE status='pending';

CREATE TABLE media_assets (
 id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 initiated_by BIGINT NOT NULL REFERENCES users(id),
 purpose TEXT NOT NULL CHECK(purpose IN ('user_avatar','brand_logo','brand_banner','product_image')),
 target_user_id BIGINT,
 brand_id BIGINT,
 product_id BIGINT,
 color_id BIGINT,
 bucket TEXT NOT NULL,
 object_key TEXT NOT NULL UNIQUE CHECK(object_key LIKE 'assets/%'),
 state TEXT NOT NULL DEFAULT 'processing' CHECK(state IN ('processing','ready','deleting','deleted')),
 mime TEXT CHECK(mime IN ('image/jpeg','image/png')),
 size_bytes BIGINT CHECK(size_bytes BETWEEN 1 AND 8388608),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 confirmed_at TIMESTAMPTZ,
 expires_at TIMESTAMPTZ NOT NULL DEFAULT now()+interval '15 minutes',
 delete_after TIMESTAMPTZ,
 CHECK(COALESCE((purpose='user_avatar' AND target_user_id=initiated_by AND brand_id IS NULL AND product_id IS NULL AND color_id IS NULL)
    OR (purpose IN ('brand_logo','brand_banner') AND target_user_id IS NULL AND brand_id>0 AND product_id IS NULL AND color_id IS NULL)
    OR (purpose='product_image' AND target_user_id IS NULL AND brand_id>0 AND product_id>0 AND color_id>0),false)),
 CHECK(state <> 'ready' OR (confirmed_at IS NOT NULL AND mime IS NOT NULL AND size_bytes IS NOT NULL))
);
CREATE INDEX media_assets_gc ON media_assets(state,expires_at,delete_after);
CREATE TABLE media_upload_requests (
 id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 asset_id BIGINT NOT NULL UNIQUE REFERENCES media_assets(id),
 initiated_by BIGINT NOT NULL REFERENCES users(id),
 expected_size BIGINT NOT NULL CHECK(expected_size BETWEEN 1 AND 5242880),
 expected_mime TEXT NOT NULL CHECK(expected_mime IN ('image/jpeg','image/png','image/webp')),
 source_size BIGINT CHECK(source_size BETWEEN 1 AND 5242880),
 image_id BIGINT,
 position SMALLINT CHECK(position BETWEEN 0 AND 9),
 state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','processing','complete','rejected')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 completed_at TIMESTAMPTZ
);
ALTER TABLE users ADD COLUMN avatar_media_id BIGINT UNIQUE REFERENCES media_assets(id),
 ADD CONSTRAINT user_avatar_source CHECK(avatar_media_id IS NULL OR avatar_url='');
ALTER TABLE brands ADD COLUMN logo_media_id BIGINT UNIQUE REFERENCES media_assets(id),
 ADD COLUMN banner_media_id BIGINT UNIQUE REFERENCES media_assets(id),
 ADD CONSTRAINT brand_logo_source CHECK(logo_media_id IS NULL OR logo_url=''),
 ADD CONSTRAINT brand_banner_source CHECK(banner_media_id IS NULL OR banner_url='');
ALTER TABLE product_color_images ALTER COLUMN url DROP NOT NULL;
ALTER TABLE product_color_images ADD COLUMN media_id BIGINT UNIQUE REFERENCES media_assets(id),
 ADD CONSTRAINT product_image_source CHECK((url IS NOT NULL AND media_id IS NULL) OR (url IS NULL AND media_id IS NOT NULL));

-- These views contain identifiers, never stored HTTP URLs. Publication follows
-- the CURRENT domain state; private images are not made public by uploading.
CREATE VIEW media_references AS
 SELECT avatar_media_id AS asset_id,'user_avatar'::text AS purpose,id AS user_id,NULL::bigint AS brand_id,NULL::bigint AS product_id,NULL::bigint AS color_id,NULL::bigint AS image_id FROM users WHERE avatar_media_id IS NOT NULL
 UNION ALL SELECT logo_media_id,'brand_logo',NULL,id,NULL,NULL,NULL FROM brands WHERE logo_media_id IS NOT NULL
 UNION ALL SELECT banner_media_id,'brand_banner',NULL,id,NULL,NULL,NULL FROM brands WHERE banner_media_id IS NOT NULL
 UNION ALL SELECT i.media_id,'product_image',NULL,p.brand_id,c.product_id,c.id,i.id FROM product_color_images i JOIN product_colors c ON c.id=i.product_color_id JOIN products p ON p.id=c.product_id WHERE i.media_id IS NOT NULL;

-- Detach queues deletion inside the SAME SQL transaction. A rollback restores
-- the reference and the old ready asset together. S3 deletion happens later.
CREATE FUNCTION media_detached() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE old_id BIGINT;
BEGIN
 old_id := (to_jsonb(OLD)->>TG_ARGV[0])::bigint;
 IF old_id IS NOT NULL AND (TG_OP='DELETE' OR old_id IS DISTINCT FROM (to_jsonb(NEW)->>TG_ARGV[0])::bigint) THEN
   UPDATE media_assets SET state='deleting',delete_after=now()+interval '5 minutes'
   WHERE id=old_id AND state='ready' AND NOT EXISTS(SELECT 1 FROM media_references WHERE asset_id=old_id);
 END IF;
 RETURN NULL;
END $$;
CREATE TRIGGER user_media_detached AFTER UPDATE OR DELETE ON users FOR EACH ROW EXECUTE FUNCTION media_detached('avatar_media_id');
CREATE TRIGGER brand_logo_detached AFTER UPDATE OR DELETE ON brands FOR EACH ROW EXECUTE FUNCTION media_detached('logo_media_id');
CREATE TRIGGER brand_banner_detached AFTER UPDATE OR DELETE ON brands FOR EACH ROW EXECUTE FUNCTION media_detached('banner_media_id');
CREATE TRIGGER product_media_detached AFTER UPDATE OR DELETE ON product_color_images FOR EACH ROW EXECUTE FUNCTION media_detached('media_id');

-- Deferred checks permit atomic ready+attach, but no committed ready orphan or
-- reference to processing/deleted/wrong-target assets. No cross-owner sharing.
CREATE FUNCTION media_check_binding() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE aid BIGINT; a media_assets; n BIGINT;
BEGIN
 IF TG_TABLE_NAME='media_assets' THEN aid:=NEW.id;
 ELSE aid:=(to_jsonb(NEW)->>TG_ARGV[0])::bigint; END IF;
 IF aid IS NULL THEN RETURN NULL; END IF;
 SELECT * INTO a FROM media_assets WHERE id=aid;
 SELECT count(*) INTO n FROM media_references WHERE asset_id=aid;
 IF a.state='ready' THEN
   IF n<>1 OR NOT EXISTS(SELECT 1 FROM media_references r WHERE r.asset_id=aid AND r.purpose=a.purpose
      AND r.user_id IS NOT DISTINCT FROM a.target_user_id AND r.brand_id IS NOT DISTINCT FROM a.brand_id
      AND r.product_id IS NOT DISTINCT FROM a.product_id AND r.color_id IS NOT DISTINCT FROM a.color_id) THEN
      RAISE EXCEPTION 'invalid media binding' USING ERRCODE='23514';
   END IF;
 ELSIF n<>0 THEN RAISE EXCEPTION 'media not ready' USING ERRCODE='23514'; END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER media_asset_binding AFTER INSERT OR UPDATE ON media_assets DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION media_check_binding();
CREATE CONSTRAINT TRIGGER user_media_binding AFTER INSERT OR UPDATE ON users DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION media_check_binding('avatar_media_id');
CREATE CONSTRAINT TRIGGER brand_logo_binding AFTER INSERT OR UPDATE ON brands DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION media_check_binding('logo_media_id');
CREATE CONSTRAINT TRIGGER brand_banner_binding AFTER INSERT OR UPDATE ON brands DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION media_check_binding('banner_media_id');
CREATE CONSTRAINT TRIGGER product_media_binding AFTER INSERT OR UPDATE ON product_color_images DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION media_check_binding('media_id');
