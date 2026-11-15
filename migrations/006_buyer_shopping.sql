CREATE TABLE favorites (
    user_id BIGINT NOT NULL REFERENCES users(id),
    product_id BIGINT NOT NULL REFERENCES products(id),
    PRIMARY KEY (user_id, product_id)
);
CREATE INDEX favorites_product_id ON favorites(product_id);

-- A NULL variant is an explicit tombstone retained after catalog deletion.
-- The management transaction detaches references before deleting variants.
CREATE TABLE cart_items (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    product_variant_id BIGINT REFERENCES product_variants(id),
    quantity BIGINT NOT NULL CHECK (quantity BETWEEN 1 AND 100),
    UNIQUE (user_id, product_variant_id)
);
CREATE INDEX cart_items_variant_id ON cart_items(product_variant_id);
