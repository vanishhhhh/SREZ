ALTER TABLE brand_pages DROP CONSTRAINT brand_pages_draft_check,
    DROP CONSTRAINT brand_pages_published_check,
    ADD CONSTRAINT brand_pages_draft_check CHECK (jsonb_typeof(draft) = 'object' AND octet_length(draft::text) <= 163840),
    ADD CONSTRAINT brand_pages_published_check CHECK (jsonb_typeof(published) = 'object' AND octet_length(published::text) <= 163840);

ALTER TABLE media_assets DROP CONSTRAINT media_assets_purpose_check,
    ADD CONSTRAINT media_assets_purpose_check CHECK (purpose IN ('user_avatar','brand_logo','brand_banner','product_image','brand_editorial_image'));

-- The original combined target check cannot accept a new purpose.
DO $$
DECLARE target_check text;
BEGIN
    SELECT conname INTO STRICT target_check FROM pg_constraint
    WHERE conrelid='media_assets'::regclass AND contype='c'
      AND pg_get_constraintdef(oid) LIKE '%target_user_id = initiated_by%';
    EXECUTE format('ALTER TABLE media_assets DROP CONSTRAINT %I', target_check);
END $$;

ALTER TABLE media_assets ADD CONSTRAINT media_assets_target_check CHECK (COALESCE(
    (purpose='user_avatar' AND target_user_id=initiated_by AND brand_id IS NULL AND product_id IS NULL AND color_id IS NULL)
 OR (purpose IN ('brand_logo','brand_banner','brand_editorial_image') AND target_user_id IS NULL AND brand_id>0 AND product_id IS NULL AND color_id IS NULL)
 OR (purpose='product_image' AND target_user_id IS NULL AND brand_id>0 AND product_id>0 AND color_id>0), false));

CREATE TABLE brand_editorial_media (
    asset_id BIGINT PRIMARY KEY REFERENCES media_assets(id),
    brand_id BIGINT NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    retain_until TIMESTAMPTZ NOT NULL DEFAULT now()+interval '7 days',
    UNIQUE (brand_id, asset_id)
);
CREATE INDEX brand_editorial_media_retention ON brand_editorial_media(retain_until, asset_id);

CREATE TABLE brand_page_media (
    brand_id BIGINT NOT NULL REFERENCES brand_pages(brand_id) ON DELETE CASCADE,
    published BOOLEAN NOT NULL,
    asset_id BIGINT NOT NULL,
    PRIMARY KEY (brand_id, published, asset_id),
    FOREIGN KEY (brand_id, asset_id) REFERENCES brand_editorial_media(brand_id, asset_id)
);
CREATE INDEX brand_page_media_asset ON brand_page_media(asset_id, published);

-- The library is the single binding recognized by the existing deferred ready
-- invariant. Snapshot references independently authorize delivery and protect retention.
CREATE OR REPLACE VIEW media_references AS
 SELECT avatar_media_id AS asset_id,'user_avatar'::text AS purpose,id AS user_id,NULL::bigint AS brand_id,NULL::bigint AS product_id,NULL::bigint AS color_id,NULL::bigint AS image_id FROM users WHERE avatar_media_id IS NOT NULL
 UNION ALL SELECT logo_media_id,'brand_logo',NULL,id,NULL,NULL,NULL FROM brands WHERE logo_media_id IS NOT NULL
 UNION ALL SELECT banner_media_id,'brand_banner',NULL,id,NULL,NULL,NULL FROM brands WHERE banner_media_id IS NOT NULL
 UNION ALL SELECT i.media_id,'product_image',NULL,p.brand_id,c.product_id,c.id,i.id FROM product_color_images i JOIN product_colors c ON c.id=i.product_color_id JOIN products p ON p.id=c.product_id WHERE i.media_id IS NOT NULL
 UNION ALL SELECT asset_id,'brand_editorial_image',NULL,brand_id,NULL,NULL,NULL FROM brand_editorial_media;

CREATE TRIGGER editorial_media_detached AFTER DELETE ON brand_editorial_media
 FOR EACH ROW EXECUTE FUNCTION media_detached('asset_id');
CREATE CONSTRAINT TRIGGER editorial_media_binding AFTER INSERT OR UPDATE ON brand_editorial_media
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION media_check_binding('asset_id');
