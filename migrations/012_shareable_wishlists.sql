-- Additive only: existing favorites, carts, orders and media are preserved.
CREATE TABLE wishlists (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    owner_user_id BIGINT NOT NULL REFERENCES users(id),
    title TEXT NOT NULL CHECK (char_length(btrim(title)) BETWEEN 1 AND 100),
    description TEXT NOT NULL DEFAULT '' CHECK (char_length(description) <= 2000),
    visibility TEXT NOT NULL DEFAULT 'private' CHECK (visibility IN ('private', 'public')),
    share_token_hash BYTEA UNIQUE,
    share_token_sealed BYTEA,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CHECK ((visibility='private' AND share_token_hash IS NULL AND share_token_sealed IS NULL)
        OR (visibility='public' AND octet_length(share_token_hash)=32 AND share_token_sealed IS NOT NULL))
);
CREATE INDEX wishlists_owner_updated ON wishlists(owner_user_id,updated_at DESC,id DESC);

CREATE TABLE wishlist_items (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    wishlist_id BIGINT NOT NULL REFERENCES wishlists(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    product_variant_id BIGINT REFERENCES product_variants(id) ON DELETE SET NULL,
    position INTEGER NOT NULL CHECK (position >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    UNIQUE (wishlist_id,product_id),
    CONSTRAINT wishlist_item_position UNIQUE (wishlist_id,position) DEFERRABLE INITIALLY IMMEDIATE
);
CREATE INDEX wishlist_items_product ON wishlist_items(product_id);
CREATE INDEX wishlist_items_variant ON wishlist_items(product_variant_id);

-- A chosen variant must belong to this product. A removed variant becomes NULL,
-- retaining the user's product selection without blocking the existing PVM.
CREATE FUNCTION wishlist_variant_matches_product() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.product_variant_id IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM product_variants v JOIN product_colors c ON c.id=v.product_color_id
        WHERE v.id=NEW.product_variant_id AND c.product_id=NEW.product_id
    ) THEN
        RAISE EXCEPTION 'invalid wishlist variant' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER wishlist_variant_product BEFORE INSERT OR UPDATE OF product_id,product_variant_id
    ON wishlist_items FOR EACH ROW EXECUTE FUNCTION wishlist_variant_matches_product();
