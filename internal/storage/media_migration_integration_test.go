package storage_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"testing"

	"srez/migrations"
)

func TestPostgresMedia009UpgradeAndBindingConstraints(t *testing.T) {
	db := testDB(t)
	exec(t, db, `CREATE TABLE schema_migrations(version TEXT PRIMARY KEY,checksum TEXT NOT NULL,applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`)
	for _, name := range []string{"001_users.sql", "002_catalog.sql", "003_auth.sql", "004_brand_memberships.sql", "005_product_image_ids.sql", "006_buyer_shopping.sql", "007_buyer_orders.sql", "008_unpaid_order_cancellation.sql", "009_media_uploads.sql"} {
		body, e := os.ReadFile("../../migrations/" + name)
		pvmCheck(t, e)
		exec(t, db, string(body))
		exec(t, db, `INSERT INTO schema_migrations(version,checksum) VALUES($1,$2)`, name, fmt.Sprintf("%x", sha256.Sum256(body)))
	}
	var uid, brand, product, color int64
	pvmCheck(t, db.Get(&uid, `INSERT INTO users(email,username,display_name,password_hash,created_at,updated_at) VALUES('legacy_media@example.com','legacy_media','Legacy media','!',now(),now()) RETURNING id`))
	pvmCheck(t, db.Get(&brand, `INSERT INTO brands(name,status,created_at,updated_at) VALUES('Legacy media brand','draft',now(),now()) RETURNING id`))
	pvmCheck(t, db.Get(&product, `INSERT INTO products(brand_id,name,price_kopecks,status,created_at,updated_at) VALUES($1,'Legacy media product',1,'draft',now(),now()) RETURNING id`, brand))
	pvmCheck(t, db.Get(&color, `INSERT INTO product_colors(product_id,color,color_key) VALUES($1,'Black','black') RETURNING id`, product))
	exec(t, db, `INSERT INTO product_color_images(product_color_id,position,url) VALUES($1,0,'https://example.com/legacy.jpg')`, color)
	for _, state := range []string{"pending", "ready"} {
		exec(t, db, `INSERT INTO media_uploads(user_id,brand_id,product_id,color_id,bucket,staging_key,object_key,expected_size,actual_size,mime,status,confirmed_at) VALUES($1,$2,$3,$4,'srez-media',$5,$6,1,CASE WHEN $7='ready' THEN 1 END,'image/jpeg',$7,CASE WHEN $7='ready' THEN now() END)`, uid, brand, product, color, "staging/legacy-"+state, "ready/legacy-"+state, state)
	}
	pvmCheck(t, migrations.Apply(context.Background(), db))
	pvmCheck(t, migrations.Apply(context.Background(), db))
	var count int
	pvmCheck(t, db.Get(&count, `SELECT count(*) FROM media_uploads WHERE status='rejected' AND object_key='ready/legacy-pending'`))
	if count != 1 {
		t.Fatal("legacy pending not retired")
	}
	pvmCheck(t, db.Get(&count, `SELECT count(*) FROM media_uploads WHERE status='ready' AND retired_at IS NOT NULL AND object_key='ready/legacy-ready'`))
	if count != 1 {
		t.Fatal("legacy ready lost controlled retention")
	}
	var url string
	pvmCheck(t, db.Get(&url, `SELECT url FROM product_color_images WHERE product_color_id=$1`, color))
	if url != "https://example.com/legacy.jpg" {
		t.Fatal("legacy image modified")
	}
	if _, e := db.Exec(`INSERT INTO media_assets(initiated_by,purpose,bucket,object_key) VALUES($1,'user_avatar','srez-media','assets/null-target')`, uid); e == nil {
		t.Fatal("NULL target accepted")
	}
	if _, e := db.Exec(`INSERT INTO media_assets(initiated_by,purpose,target_user_id,bucket,object_key,state,mime,size_bytes,confirmed_at) VALUES($1,'user_avatar',$1,'srez-media','assets/orphan','ready','image/jpeg',1,now())`, uid); e == nil {
		t.Fatal("ready orphan accepted")
	}
}
