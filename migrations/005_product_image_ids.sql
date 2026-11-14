-- Keep existing URLs/order and give each image a stable management identity.
ALTER TABLE product_color_images
    ADD COLUMN id BIGINT GENERATED ALWAYS AS IDENTITY,
    ADD CONSTRAINT product_color_images_id_unique UNIQUE (id);

-- Reordering can temporarily swap occupied positions, but must be unique at
-- the end of the transaction. Existing range/URL/FK constraints stay intact.
ALTER TABLE product_color_images
    DROP CONSTRAINT product_color_images_pkey,
    ADD CONSTRAINT product_color_images_pkey
        PRIMARY KEY (product_color_id, position) DEFERRABLE INITIALLY IMMEDIATE;
