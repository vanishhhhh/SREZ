-- Unpublished migration: 001-007 remain unchanged. Stop old API instances
-- before upgrading; old unpaid debits become explicit, expiring reservations.
LOCK TABLE brands IN EXCLUSIVE MODE;
ALTER TABLE orders DROP CONSTRAINT orders_status_check;
ALTER TABLE orders ADD CONSTRAINT orders_status_check CHECK (status IN ('unpaid','cancelled','expired'));
ALTER TABLE orders ADD COLUMN cancel_before TIMESTAMPTZ;
UPDATE orders SET cancel_before=created_at+INTERVAL '15 minutes';
ALTER TABLE orders ALTER COLUMN cancel_before SET NOT NULL;
ALTER TABLE orders ADD COLUMN cancelled_at TIMESTAMPTZ;
ALTER TABLE orders ADD COLUMN expired_at TIMESTAMPTZ;
ALTER TABLE orders ADD CONSTRAINT orders_cancellation_check CHECK (
    cancel_before=created_at+INTERVAL '15 minutes' AND
    ((status='unpaid' AND cancelled_at IS NULL AND expired_at IS NULL) OR
     (status='cancelled' AND cancelled_at IS NOT NULL AND cancelled_at>=created_at AND cancelled_at<cancel_before AND expired_at IS NULL) OR
     (status='expired' AND expired_at IS NOT NULL AND expired_at>=cancel_before AND cancelled_at IS NULL))
);
CREATE INDEX orders_due_reservations ON orders(cancel_before,id) WHERE status='unpaid';

CREATE TABLE order_reservations (
    order_id BIGINT NOT NULL,
    -- Historical identity: deletion of a catalog variant must not destroy
    -- the order or resurrect the deleted variant when the hold is released.
    product_variant_id BIGINT NOT NULL,
    quantity BIGINT NOT NULL CHECK (quantity BETWEEN 1 AND 100),
    reserved_at TIMESTAMPTZ NOT NULL,
    released_at TIMESTAMPTZ CHECK (released_at>=reserved_at),
    PRIMARY KEY(order_id,product_variant_id),
    FOREIGN KEY(order_id,product_variant_id) REFERENCES order_items(order_id,product_variant_id)
);
CREATE INDEX order_reservations_active_variant ON order_reservations(product_variant_id) WHERE released_at IS NULL;
-- These quantities were already deducted by checkout under 007. Do not deduct
-- or return them again during migration; cancellation/expiry releases once.
INSERT INTO order_reservations(order_id,product_variant_id,quantity,reserved_at)
SELECT i.order_id,i.product_variant_id,i.quantity,o.created_at
FROM order_items i JOIN orders o ON o.id=i.order_id WHERE o.status='unpaid';
