CREATE TABLE catalog_search_revision (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    namespace uuid NOT NULL DEFAULT gen_random_uuid(),
    revision bigint NOT NULL DEFAULT 1
);
INSERT INTO catalog_search_revision DEFAULT VALUES;

-- The revision advances in the same transaction as the catalog change.
CREATE FUNCTION bump_catalog_search_revision() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    UPDATE catalog_search_revision SET revision = revision + 1 WHERE singleton;
    RETURN NULL;
END;
$$;

CREATE TRIGGER catalog_search_products
AFTER INSERT OR DELETE OR TRUNCATE OR UPDATE OF brand_id, category, price_kopecks, status, created_at ON products
FOR EACH STATEMENT EXECUTE FUNCTION bump_catalog_search_revision();

CREATE TRIGGER catalog_search_brands
AFTER INSERT OR DELETE OR TRUNCATE OR UPDATE OF status ON brands
FOR EACH STATEMENT EXECUTE FUNCTION bump_catalog_search_revision();

CREATE TRIGGER catalog_search_colors
AFTER INSERT OR DELETE OR TRUNCATE OR UPDATE OF product_id, color_key ON product_colors
FOR EACH STATEMENT EXECUTE FUNCTION bump_catalog_search_revision();

CREATE TRIGGER catalog_search_variants
AFTER INSERT OR DELETE OR TRUNCATE OR UPDATE OF product_color_id, size_key, stock ON product_variants
FOR EACH STATEMENT EXECUTE FUNCTION bump_catalog_search_revision();
