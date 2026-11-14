package storage_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"srez/internal/devseed"
	"srez/internal/entities"
	"srez/internal/storage"
	"srez/internal/usecase"
	"srez/migrations"
)

// Only newly created, random schemas are changed or removed. The URL must point
// at a dedicated test DB; never run integration tests against production.
func testDB(t *testing.T) *sqlx.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set; PostgreSQL integration test skipped")
	}
	admin, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("catalog_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec(`CREATE SCHEMA ` + pq.QuoteIdentifier(schema)); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(`DROP SCHEMA ` + pq.QuoteIdentifier(schema) + ` CASCADE`); err != nil {
			t.Error(err)
		}
		admin.Close()
	})
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := sqlx.Connect("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
func exec(t *testing.T, db *sqlx.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatal(err)
	}
}
func count(t *testing.T, db *sqlx.DB, table string) int {
	t.Helper()
	var n int
	if err := db.Get(&n, `SELECT count(*) FROM `+pq.QuoteIdentifier(table)); err != nil {
		t.Fatal(err)
	}
	return n
}
func migrate(t *testing.T, db *sqlx.DB) {
	t.Helper()
	if err := migrations.Apply(context.Background(), db); err != nil {
		t.Fatal(err)
	}
}
func seed(t *testing.T, db *sqlx.DB) {
	t.Helper()
	if err := devseed.Apply(context.Background(), db); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresMigrations(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprintf("existing_users_%v", existing), func(t *testing.T) {
			db := testDB(t)
			if existing {
				body, err := os.ReadFile("../../migrations/001_users.sql")
				if err != nil {
					t.Fatal(err)
				}
				exec(t, db, string(body))
				exec(t, db, `INSERT INTO users(email,username,display_name,password_hash,created_at,updated_at) VALUES('kept@example.com','kept_user','Сохранён','!',now(),now())`)
			}
			// Two simultaneous app starters must both succeed.
			var wg sync.WaitGroup
			errs := make(chan error, 2)
			for range 2 {
				wg.Add(1)
				go func() { defer wg.Done(); errs <- migrations.Apply(context.Background(), db) }()
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				if err != nil {
					t.Fatal(err)
				}
			}
			migrate(t, db)
			if count(t, db, "schema_migrations") != 5 {
				t.Fatal("wrong migration count")
			}
			if existing && count(t, db, "users") != 1 {
				t.Fatal("existing user lost")
			}
			seed(t, db)
			seed(t, db)
			if count(t, db, "brands") != 2 || count(t, db, "products") != 2 || count(t, db, "product_colors") != 4 || count(t, db, "product_variants") != 6 || count(t, db, "product_color_images") != 6 {
				t.Fatal("seed duplicated or incomplete")
			}
			exec(t, db, `UPDATE schema_migrations SET checksum='tampered' WHERE version='002_catalog.sql'`)
			if err := migrations.Apply(context.Background(), db); err == nil {
				t.Fatal("modified migration accepted")
			}
		})
	}
}
func TestPostgresMigrationRollback(t *testing.T) {
	db := testDB(t)
	exec(t, db, `CREATE TABLE brands(sentinel TEXT)`)
	if err := migrations.Apply(context.Background(), db); err == nil {
		t.Fatal("expected migration failure")
	}
	var exists bool
	if err := db.Get(&exists, `SELECT to_regclass('users') IS NOT NULL`); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("failed batch was not rolled back")
	}
	exec(t, db, `INSERT INTO brands VALUES('kept')`)
}
func TestPostgresCatalog(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	migrate(t, db)
	seed(t, db)
	brands := storage.NewBrands(db)
	repo := storage.NewProducts(db)
	products := usecase.NewProducts(repo, brands)
	list, err := brands.ListActive(ctx)
	if err != nil || len(list) != 2 {
		t.Fatalf("brands: %v %v", list, err)
	}
	for _, brand := range list {
		got, err := brands.GetActiveBySlug(ctx, brand.Slug())
		if err != nil || got == nil || got.ID() != brand.ID() {
			t.Fatalf("brand read: %v %v", got, err)
		}
	}
	cards, err := products.List(ctx)
	if err != nil || len(cards) != 2 {
		t.Fatalf("cards: %v %v", cards, err)
	}
	for _, card := range cards {
		detail, err := products.Get(ctx, card.ID())
		if err != nil {
			t.Fatal(err)
		}
		if len(detail.Publication.Colors) != 2 || len(detail.Publication.Variants) != 3 {
			t.Fatal("missing children")
		}
		if err = detail.Product.ValidatePublication(detail.Publication); err != nil {
			t.Fatal(err)
		}
		black := detail.Publication.Colors[0]
		images := black.Images()
		if len(images) != 2 || !strings.HasSuffix(images[0], "black.png") || !strings.HasSuffix(images[1], "black-back.png") {
			t.Fatalf("image order %v", images)
		}
		if detail.Publication.Variants[0].ProductColorID() != black.ID() || detail.Publication.Variants[1].ProductColorID() != black.ID() || detail.Publication.Variants[2].ProductColorID() != detail.Publication.Colors[1].ID() || detail.Publication.Variants[1].Stock() != 0 {
			t.Fatal("variant/color relation or zero stock lost")
		}
	}
	b := list[0]
	draft, err := products.Create(ctx, b.ID(), " Черновик ", "", 0, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if draft.ID() <= cards[len(cards)-1].ID() || draft.Status() != entities.ProductDraft || draft.Name() != "Черновик" {
		t.Fatal("invalid saved draft")
	}
	var status string
	if err = db.Get(&status, `SELECT status FROM products WHERE id=$1`, draft.ID()); err != nil || status != "draft" {
		t.Fatal("draft not persisted")
	}
	if _, err = products.Get(ctx, draft.ID()); !errors.Is(err, usecase.ErrProductNotFound) {
		t.Fatalf("draft exposed: %v", err)
	}
	cards2, err := products.List(ctx)
	if err != nil || len(cards2) != 2 {
		t.Fatal("draft in list")
	}
	if _, err = products.Create(ctx, 9223372036854775807, "Товар", "", 0, "", nil); !errors.Is(err, usecase.ErrBrandNotFound) {
		t.Fatal(err)
	}
	if _, err = products.Create(ctx, 0, "Товар", "", 0, "", nil); !errors.Is(err, entities.ErrInvalidValue) {
		t.Fatal(err)
	}
	for _, status := range []string{"draft", "suspended"} {
		exec(t, db, `UPDATE brands SET status=$2 WHERE id=$1`, b.ID(), status)
		if _, err = products.Create(ctx, b.ID(), "Товар", "", 0, "", nil); !errors.Is(err, usecase.ErrBrandInactive) {
			t.Fatal(err)
		}
		if got, err := brands.GetActiveBySlug(ctx, b.Slug()); err != nil || got != nil {
			t.Fatal("private brand exposed")
		}
		if got, err := products.List(ctx); err != nil || len(got) != 1 {
			t.Fatal("private brand products exposed")
		}
		if _, err = products.Get(ctx, cards[0].ID()); !errors.Is(err, usecase.ErrProductNotFound) {
			t.Fatal("private brand product detail exposed")
		}
	}
	exec(t, db, `UPDATE brands SET status='active' WHERE id=$1`, b.ID())
	exec(t, db, `UPDATE products SET status='hidden' WHERE id=$1`, cards[0].ID())
	if _, err = products.Get(ctx, cards[0].ID()); !errors.Is(err, usecase.ErrProductNotFound) {
		t.Fatal("hidden product exposed")
	}
	// Repository enforces the brand check even without the preliminary usecase read.
	transient, err := entities.NewProduct(b.ID(), "Product", "", 0, "", nil, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	exec(t, db, `UPDATE brands SET status='suspended' WHERE id=$1`, b.ID())
	if _, err = repo.InsertDraft(ctx, *transient); !errors.Is(err, usecase.ErrBrandInactive) {
		t.Fatal("atomic status check absent")
	}
}
func TestPostgresConstraints(t *testing.T) {
	db := testDB(t)
	migrate(t, db)
	seed(t, db)
	var productID, colorID, brandID int64
	if err := db.Get(&productID, `SELECT min(id) FROM products`); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&brandID, `SELECT min(id) FROM brands`); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&colorID, `SELECT min(id) FROM product_colors`); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, q string
		args    []any
	}{
		{"duplicate slug", `UPDATE brands SET slug='srez-studio' WHERE slug='linea'`, nil},
		{"duplicate color", `INSERT INTO product_colors(product_id,color,color_key) VALUES($1,' ЧЁРНЫЙ ','чёрный')`, []any{productID}},
		{"duplicate size", `INSERT INTO product_variants(product_color_id,size,size_key,stock) VALUES($1,'m','m',1)`, []any{colorID}},
		{"duplicate image position", `INSERT INTO product_color_images(product_color_id,position,url) VALUES($1,0,'https://example.com/another')`, []any{colorID}},
		{"duplicate image url", `INSERT INTO product_color_images SELECT product_color_id,9,url FROM product_color_images WHERE product_color_id=$1 AND position=0`, []any{colorID}},
		{"too many image slots", `INSERT INTO product_color_images(product_color_id,position,url) VALUES($1,10,'https://example.com/another')`, []any{colorID}},
		{"negative stock", `UPDATE product_variants SET stock=-1 WHERE product_color_id=$1`, []any{colorID}},
		{"negative price", `UPDATE products SET price_kopecks=-1 WHERE id=$1`, []any{productID}},
		{"missing brand FK", `UPDATE products SET brand_id=9223372036854775807 WHERE id=$1`, []any{productID}},
		{"missing color FK", `INSERT INTO product_variants(product_color_id,size,size_key,stock) VALUES(9223372036854775807,'M','m',1)`, nil},
		{"bad status", `UPDATE brands SET status='unknown' WHERE id=$1`, []any{brandID}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := db.Exec(tt.q, tt.args...)
			var pgErr *pq.Error
			if !errors.As(err, &pgErr) || !strings.HasPrefix(string(pgErr.Code), "23") {
				t.Fatalf("expected constraint error: %v", err)
			}
		})
	}
	// Domain permits multiple brands without a slug, including Unicode names.
	exec(t, db, `INSERT INTO brands(name,status,created_at,updated_at) VALUES('  日本語  ','draft',now(),now()),('Бренд','draft',now(),now())`)
}
func TestPostgresSeedPreservesExisting(t *testing.T) {
	db := testDB(t)
	migrate(t, db)
	exec(t, db, `INSERT INTO brands(name,slug,status,created_at,updated_at) VALUES('Existing','srez-studio','draft',now(),now())`)
	seed(t, db)
	seed(t, db)
	var name string
	if err := db.Get(&name, `SELECT name FROM brands WHERE slug='srez-studio'`); err != nil || name != "Existing" {
		t.Fatal("existing brand overwritten")
	}
	if count(t, db, "products") != 1 {
		t.Fatal("seed attached products to existing brand")
	}
}
func TestPostgresUserCRUDCompatibility(t *testing.T) {
	db := testDB(t)
	migrate(t, db)
	seed(t, db)
	ctx := context.Background()
	users := usecase.NewUsers(storage.NewUsers(db))
	user, err := users.Create(ctx, "test@example.com", "integration_user", "Тест")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = users.Create(ctx, "other@example.com", "integration_user", "Другой"); !errors.Is(err, usecase.ErrUsernameTaken) {
		t.Fatal(err)
	}
	got, err := users.Get(ctx, user.ID())
	if err != nil || got.Username() != user.Username() {
		t.Fatal("get user")
	}
	list, err := users.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatal("list users")
	}
	name, avatar := "Новое имя", "https://example.com/avatar.png"
	got, err = users.Update(ctx, user.ID(), &name, &avatar)
	if err != nil || got.DisplayName() != name || got.AvatarURL() != avatar {
		t.Fatal("update user")
	}
	migrate(t, db)
	got, err = users.Get(ctx, user.ID())
	if err != nil || got.DisplayName() != name {
		t.Fatal("migration changed user")
	}
	if err = users.Delete(ctx, user.ID()); err != nil {
		t.Fatal(err)
	}
	if _, err = users.Get(ctx, user.ID()); !errors.Is(err, usecase.ErrUserNotFound) {
		t.Fatal("delete user")
	}
}
