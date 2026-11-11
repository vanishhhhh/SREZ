CREATE TABLE brands (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    logo_url TEXT NOT NULL DEFAULT '',
    banner_url TEXT NOT NULL DEFAULT '',
    contact_email TEXT NOT NULL DEFAULT '',
    telegram_url TEXT NOT NULL DEFAULT '',
    website_url TEXT NOT NULL DEFAULT '',
    accent_color TEXT NOT NULL DEFAULT '',
    background_color TEXT NOT NULL DEFAULT '',
    text_color TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('draft', 'active', 'suspended')),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL CHECK (updated_at >= created_at)
);
-- Draft brands may have no slug; only assigned slugs must be unique.
CREATE UNIQUE INDEX brands_slug_unique ON brands(slug) WHERE slug <> '';
CREATE INDEX brands_public ON brands(id) WHERE status = 'active';

CREATE TABLE products (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    brand_id BIGINT NOT NULL REFERENCES brands(id),
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    price_kopecks BIGINT NOT NULL CHECK (price_kopecks >= 0),
    category TEXT NOT NULL DEFAULT '' CHECK (category IN ('', 'tshirt', 'hoodie', 'pants', 'outerwear', 'footwear', 'accessories', 'other')),
    materials TEXT[] NOT NULL DEFAULT '{}',
    status TEXT NOT NULL CHECK (status IN ('draft', 'published', 'hidden')),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL CHECK (updated_at >= created_at)
);
CREATE INDEX products_brand_id ON products(brand_id);
CREATE INDEX products_public ON products(id, brand_id) WHERE status = 'published';

CREATE TABLE product_colors (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    product_id BIGINT NOT NULL REFERENCES products(id),
    color TEXT NOT NULL,
    -- Keys use Go strings.ToLower(strings.TrimSpace(value)), exactly as entities.
    -- Stored explicitly to avoid differences in PostgreSQL locale/Unicode rules.
    color_key TEXT NOT NULL CHECK (color_key <> ''),
    UNIQUE (product_id, color_key)
);
CREATE TABLE product_color_images (
    product_color_id BIGINT NOT NULL REFERENCES product_colors(id),
    position SMALLINT NOT NULL CHECK (position BETWEEN 0 AND 9),
    url TEXT NOT NULL CHECK (url <> ''),
    PRIMARY KEY (product_color_id, position),
    UNIQUE (product_color_id, url)
);
CREATE TABLE product_variants (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    product_color_id BIGINT NOT NULL REFERENCES product_colors(id),
    size TEXT NOT NULL,
    size_key TEXT NOT NULL CHECK (size_key <> ''),
    stock BIGINT NOT NULL CHECK (stock >= 0),
    sku TEXT NOT NULL DEFAULT '',
    UNIQUE (product_color_id, size_key)
);
