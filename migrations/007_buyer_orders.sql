CREATE TABLE orders (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    status TEXT NOT NULL CHECK (status = 'unpaid'),
    total_kopecks BIGINT NOT NULL CHECK (total_kopecks > 0),
    created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX orders_buyer_id ON orders(user_id, id DESC);

-- Catalog IDs are historical values, deliberately without live catalog FKs.
-- Order history must survive catalog deletion and changes.
CREATE TABLE order_items (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id BIGINT NOT NULL REFERENCES orders(id),
    product_variant_id BIGINT NOT NULL CHECK (product_variant_id > 0),
    product_id BIGINT NOT NULL CHECK (product_id > 0),
    brand_id BIGINT NOT NULL CHECK (brand_id > 0),
    product_name TEXT NOT NULL CHECK (product_name <> ''),
    color TEXT NOT NULL CHECK (color <> ''),
    size TEXT NOT NULL CHECK (size <> ''),
    quantity BIGINT NOT NULL CHECK (quantity BETWEEN 1 AND 100),
    unit_price_kopecks BIGINT NOT NULL CHECK (unit_price_kopecks > 0),
    subtotal_kopecks BIGINT NOT NULL CHECK (subtotal_kopecks = unit_price_kopecks * quantity),
    UNIQUE(order_id, product_variant_id)
);
CREATE INDEX order_items_order_id ON order_items(order_id, id);
