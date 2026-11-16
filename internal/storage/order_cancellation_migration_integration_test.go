package storage_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"srez/internal/storage"
	"srez/internal/usecase"
	"srez/migrations"
)

func auditLegacyOrder(t *testing.T, db *sqlx.DB) (int64, int64, int64, int64) {
	t.Helper()
	exec(t, db, `CREATE TABLE schema_migrations(version TEXT PRIMARY KEY,checksum TEXT NOT NULL,applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`)
	for _, name := range []string{"001_users.sql", "002_catalog.sql", "003_auth.sql", "004_brand_memberships.sql", "005_product_image_ids.sql", "006_buyer_shopping.sql", "007_buyer_orders.sql"} {
		body, err := os.ReadFile("../../migrations/" + name)
		pvmCheck(t, err)
		exec(t, db, string(body))
		exec(t, db, `INSERT INTO schema_migrations(version,checksum)VALUES($1,$2)`, name, fmt.Sprintf("%x", sha256.Sum256(body)))
	}
	seed(t, db)
	// This fixture intentionally predates media migration 010; use its historical
	// SQL shape instead of today's repository projection containing media columns.
	var uid int64
	pvmCheck(t, db.Get(&uid, `INSERT INTO users(email,username,display_name,password_hash,created_at,updated_at) VALUES('legacy_order_buyer@example.com','legacy_order_buyer','Legacy buyer','!',now(),now()) RETURNING id`))
	var v, stock int64
	pvmCheck(t, db.Get(&v, `SELECT min(id) FROM product_variants WHERE stock>=2`))
	pvmCheck(t, db.Get(&stock, `SELECT stock FROM product_variants WHERE id=$1`, v))
	var id int64
	pvmCheck(t, db.Get(&id, `INSERT INTO orders(user_id,status,total_kopecks,created_at) SELECT $1,'unpaid',p.price_kopecks*2,now() FROM product_variants v JOIN product_colors c ON c.id=v.product_color_id JOIN products p ON p.id=c.product_id WHERE v.id=$2 RETURNING id`, uid, v))
	exec(t, db, `INSERT INTO order_items(order_id,product_variant_id,product_id,brand_id,product_name,color,size,quantity,unit_price_kopecks,subtotal_kopecks) SELECT $1,v.id,p.id,p.brand_id,p.name,c.color,v.size,2,p.price_kopecks,p.price_kopecks*2 FROM product_variants v JOIN product_colors c ON c.id=v.product_color_id JOIN products p ON p.id=c.product_id WHERE v.id=$2`, id, v)
	exec(t, db, `UPDATE product_variants SET stock=stock-2 WHERE id=$1`, v)
	return uid, id, v, stock
}
func TestPostgresAuditMigrationAdoptsLegacyReservationsOnce(t *testing.T) {
	db := testDB(t)
	uid, id, v, stock := auditLegacyOrder(t, db)
	pvmCheck(t, migrations.Apply(context.Background(), db))
	var got int64
	pvmCheck(t, db.Get(&got, `SELECT stock FROM product_variants WHERE id=$1`, v))
	if got != stock-2 {
		t.Fatal("migration changed already reserved stock")
	}
	pvmCheck(t, migrations.Apply(context.Background(), db))
	orders := usecase.NewOrders(storage.NewOrders(db))
	first, err := orders.Cancel(context.Background(), uid, id)
	pvmCheck(t, err)
	second, err := orders.Cancel(context.Background(), uid, id)
	pvmCheck(t, err)
	if first.Status() != "cancelled" || !first.CancelledAt().Equal(*second.CancelledAt()) {
		t.Fatal("legacy cancellation invalid")
	}
	pvmCheck(t, db.Get(&got, `SELECT stock FROM product_variants WHERE id=$1`, v))
	if got != stock {
		t.Fatal("migration/repeat cancel double-restocked")
	}
}

func TestPostgresReservationReleaseOverflowRollsBack(t *testing.T) {
	db := testDB(t)
	uid, id, v, _ := auditLegacyOrder(t, db)
	pvmCheck(t, migrations.Apply(context.Background(), db))
	exec(t, db, `UPDATE product_variants SET stock=9223372036854775807 WHERE id=$1`, v)
	if _, err := usecase.NewOrders(storage.NewOrders(db)).Cancel(context.Background(), uid, id); !errors.Is(err, usecase.ErrInventoryConflict) {
		t.Fatal("inventory overflow accepted", err)
	}
	var stock int64
	pvmCheck(t, db.Get(&stock, `SELECT stock FROM product_variants WHERE id=$1`, v))
	if stock != 9223372036854775807 {
		t.Fatal("failed release changed stock")
	}
	var n int
	pvmCheck(t, db.Get(&n, `SELECT count(*) FROM order_reservations WHERE order_id=$1 AND released_at IS NULL`, id))
	if n != 1 {
		t.Fatal("failed release marked released")
	}
	var status string
	pvmCheck(t, db.Get(&status, `SELECT status FROM orders WHERE id=$1`, id))
	if status != "unpaid" {
		t.Fatal("failed release changed order")
	}
}
func TestPostgresExpiredLegacyReservationReleasedOnce(t *testing.T) {
	db := testDB(t)
	uid, id, v, stock := auditLegacyOrder(t, db)
	exec(t, db, `UPDATE orders SET created_at=now()-interval '16 minutes' WHERE id=$1`, id)
	pvmCheck(t, migrations.Apply(context.Background(), db))
	orders := usecase.NewOrders(storage.NewOrders(db))
	n, err := orders.Expire(context.Background(), time.Now().UTC(), 100)
	pvmCheck(t, err)
	if n != 1 {
		t.Fatal(n)
	}
	n, err = orders.Expire(context.Background(), time.Now().UTC(), 100)
	pvmCheck(t, err)
	if n != 0 {
		t.Fatal("repeat expiry released twice")
	}
	order, err := orders.Get(context.Background(), uid, id)
	pvmCheck(t, err)
	if order.Status() != "expired" || order.ExpiredAt() == nil {
		t.Fatal("legacy expiry missing")
	}
	var got int64
	pvmCheck(t, db.Get(&got, `SELECT stock FROM product_variants WHERE id=$1`, v))
	if got != stock {
		t.Fatal("legacy release incorrect")
	}
}
