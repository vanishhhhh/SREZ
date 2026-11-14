package storage_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"srez/internal/entities"
	"srez/internal/storage"
	"srez/internal/usecase"
	"srez/migrations"
)

type pvmFixture struct {
	db                                           *sqlx.DB
	m                                            *usecase.Management
	owner, brand, product, color, variant, image int64
}

func pvmCheck(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func newPVMFixture(t *testing.T, published bool) pvmFixture {
	t.Helper()
	db := testDB(t)
	migrate(t, db)
	ctx := context.Background()
	users := usecase.NewUsers(storage.NewUsers(db))
	uid := managedUser(t, users, "pvm_test_owner")
	m := usecase.NewManagement(storage.NewManagement(db))
	b, err := m.CreateBrand(ctx, uid, "PVM Brand", "pvm-test-brand")
	pvmCheck(t, err)
	p, err := entities.NewProduct(b.ID(), "PVM Product", "Description", 10000, entities.CategoryHoodie, []string{"cotton"}, time.Now().UTC())
	pvmCheck(t, err)
	p, err = m.CreateProduct(ctx, uid, *p)
	pvmCheck(t, err)
	c, err := m.CreateColor(ctx, uid, p.ID(), "Black")
	pvmCheck(t, err)
	i, err := m.CreateImage(ctx, uid, p.ID(), c.ID(), "https://example.com/front.jpg", nil)
	pvmCheck(t, err)
	v, err := m.CreateVariant(ctx, uid, p.ID(), c.ID(), "One Size", 1, "")
	pvmCheck(t, err)
	if published {
		desc, logo := "Complete brand", "https://example.com/logo.jpg"
		_, err = m.ChangeBrand(ctx, uid, b.ID(), usecase.BrandPatch{Description: &desc, LogoURL: &logo}, "patch")
		pvmCheck(t, err)
		_, err = m.ChangeBrand(ctx, uid, b.ID(), usecase.BrandPatch{}, "publish")
		pvmCheck(t, err)
		_, err = m.ChangeProduct(ctx, uid, p.ID(), usecase.ProductPatch{}, "publish")
		pvmCheck(t, err)
	}
	return pvmFixture{db, m, uid, b.ID(), p.ID(), c.ID(), v.ID(), i.ID()}
}

func TestPostgresPVMUpgradePreservesExistingImages(t *testing.T) {
	db := testDB(t)
	exec(t, db, `CREATE TABLE schema_migrations(version TEXT PRIMARY KEY,checksum TEXT NOT NULL,applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`)
	for _, name := range []string{"001_users.sql", "002_catalog.sql", "003_auth.sql", "004_brand_memberships.sql"} {
		body, err := os.ReadFile("../../migrations/" + name)
		pvmCheck(t, err)
		exec(t, db, string(body))
		exec(t, db, `INSERT INTO schema_migrations(version,checksum) VALUES($1,$2)`, name, fmt.Sprintf("%x", sha256.Sum256(body)))
	}
	seed(t, db)
	type imageSnapshot struct {
		ColorID  int64  `db:"product_color_id"`
		Position int    `db:"position"`
		URL      string `db:"url"`
	}
	var before, after []imageSnapshot
	query := `SELECT product_color_id,position,url FROM product_color_images ORDER BY product_color_id,position`
	pvmCheck(t, db.Select(&before, query))
	pvmCheck(t, migrations.Apply(context.Background(), db))
	pvmCheck(t, db.Select(&after, query))
	if len(before) == 0 || !reflect.DeepEqual(before, after) {
		t.Fatal("image migration changed existing data")
	}
	var total, unique int
	pvmCheck(t, db.Get(&total, `SELECT count(*) FROM product_color_images WHERE id>0`))
	pvmCheck(t, db.Get(&unique, `SELECT count(DISTINCT id) FROM product_color_images`))
	if total != len(before) || unique != len(before) {
		t.Fatal("missing or duplicate image identities")
	}
	pvmCheck(t, migrations.Apply(context.Background(), db))
}

func concurrentPVM(t *testing.T, n int, fn func(int) error, loser error) {
	t.Helper()
	start := make(chan struct{})
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; errs <- fn(i) }()
	}
	close(start)
	wg.Wait()
	close(errs)
	success := 0
	for err := range errs {
		if err == nil {
			success++
		} else if !errors.Is(err, loser) {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatalf("got %d successful writes, want 1", success)
	}
}
func TestPostgresPVMConcurrentUniqueness(t *testing.T) {
	f := newPVMFixture(t, false)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	concurrentPVM(t, 6, func(i int) error {
		name := "WHITE"
		if i%2 == 0 {
			name = " White "
		}
		_, err := f.m.CreateColor(ctx, f.owner, f.product, name)
		return err
	}, usecase.ErrConflict)
	concurrentPVM(t, 6, func(i int) error {
		size := "M"
		if i%2 == 0 {
			size = " m "
		}
		_, err := f.m.CreateVariant(ctx, f.owner, f.product, f.color, size, 1, "SAME-SKU")
		return err
	}, usecase.ErrConflict)
	concurrentPVM(t, 4, func(int) error {
		_, err := f.m.CreateImage(ctx, f.owner, f.product, f.color, "https://example.com/shared.jpg", nil)
		return err
	}, usecase.ErrConflict)
	colors, err := f.m.ProductColors(ctx, f.owner, f.product)
	pvmCheck(t, err)
	variants, err := f.m.ProductVariants(ctx, f.owner, f.product, f.color)
	pvmCheck(t, err)
	images, err := f.m.ProductImages(ctx, f.owner, f.product, f.color)
	pvmCheck(t, err)
	if len(colors) != 2 || len(variants) != 2 || len(images) != 2 {
		t.Fatal("concurrent duplicates committed")
	}
	// SKU is intentionally not globally unique.
	_, err = f.m.CreateVariant(ctx, f.owner, f.product, f.color, "44 EU", 0, "SAME-SKU")
	pvmCheck(t, err)
}
func TestPostgresPVMPublishedConcurrentDeletesRollback(t *testing.T) {
	for _, kind := range []string{"images", "variants"} {
		t.Run(kind, func(t *testing.T) {
			f := newPVMFixture(t, true)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if kind == "images" {
				second, err := f.m.CreateImage(ctx, f.owner, f.product, f.color, "https://example.com/back.jpg", nil)
				pvmCheck(t, err)
				ids := []int64{f.image, second.ID()}
				concurrentPVM(t, 2, func(i int) error { return f.m.DeleteImage(ctx, f.owner, f.product, f.color, ids[i]) }, entities.ErrNotReady)
				images, err := f.m.ProductImages(ctx, f.owner, f.product, f.color)
				pvmCheck(t, err)
				if len(images) != 1 || images[0].Position() != 0 {
					t.Fatal("invalid images after race")
				}
			} else {
				second, err := f.m.CreateVariant(ctx, f.owner, f.product, f.color, "M", 0, "")
				pvmCheck(t, err)
				ids := []int64{f.variant, second.ID()}
				concurrentPVM(t, 2, func(i int) error { return f.m.DeleteVariant(ctx, f.owner, f.product, f.color, ids[i]) }, entities.ErrNotReady)
				variants, err := f.m.ProductVariants(ctx, f.owner, f.product, f.color)
				pvmCheck(t, err)
				if len(variants) != 1 {
					t.Fatal("last variant removed")
				}
			}
			detail, err := storage.NewProducts(f.db).GetPublic(ctx, f.product)
			pvmCheck(t, err)
			if detail == nil {
				t.Fatal("published product lost")
			}
		})
	}
}
func TestPostgresPVMConcurrentReorderKeepsStableIDs(t *testing.T) {
	f := newPVMFixture(t, true)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	second, err := f.m.CreateImage(ctx, f.owner, f.product, f.color, "https://example.com/back.jpg", nil)
	pvmCheck(t, err)
	third, err := f.m.CreateImage(ctx, f.owner, f.product, f.color, "https://example.com/side.jpg", nil)
	pvmCheck(t, err)
	orders := [][]int64{{third.ID(), f.image, second.ID()}, {second.ID(), third.ID(), f.image}}
	errs := make(chan error, 2)
	for _, order := range orders {
		go func() { _, err := f.m.ReorderImages(ctx, f.owner, f.product, f.color, order); errs <- err }()
	}
	for range 2 {
		pvmCheck(t, <-errs)
	}
	images, err := f.m.ProductImages(ctx, f.owner, f.product, f.color)
	pvmCheck(t, err)
	seen := map[int64]bool{}
	for pos, image := range images {
		if image.Position() != pos || seen[image.ID()] {
			t.Fatal("invalid reorder")
		}
		seen[image.ID()] = true
	}
	if len(images) != 3 || !seen[f.image] || !seen[second.ID()] || !seen[third.ID()] {
		t.Fatal("reorder changed identities")
	}
	_, err = storage.NewProducts(f.db).GetPublic(ctx, f.product)
	pvmCheck(t, err)
}
func TestPostgresPVMWaitingEditorRechecksRevokedMembership(t *testing.T) {
	f := newPVMFixture(t, false)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	editor := managedUser(t, usecase.NewUsers(storage.NewUsers(f.db)), "pvm_wait_editor")
	_, err := f.m.Invite(ctx, f.owner, f.brand, "pvm_wait_editor")
	pvmCheck(t, err)
	_, err = f.m.Invitation(ctx, editor, f.brand, true)
	pvmCheck(t, err)
	guard, err := f.db.BeginTxx(ctx, nil)
	pvmCheck(t, err)
	defer guard.Rollback()
	var guardPID, id int
	pvmCheck(t, guard.GetContext(ctx, &guardPID, `SELECT pg_backend_pid()`))
	pvmCheck(t, guard.GetContext(ctx, &id, `SELECT id FROM brands WHERE id=$1 FOR UPDATE`, f.brand))
	stock := int64(9)
	result := make(chan error, 1)
	go func() {
		_, err := f.m.ChangeVariant(ctx, editor, f.product, f.color, f.variant, usecase.VariantPatch{Stock: &stock})
		result <- err
	}()
	waiting := false
	for attempt := 0; attempt < 200; attempt++ {
		pvmCheck(t, f.db.GetContext(ctx, &waiting, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity a WHERE $1=ANY(pg_blocking_pids(a.pid)))`, guardPID))
		if waiting {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("editor did not wait for brand lock")
	}
	_, err = guard.ExecContext(ctx, `UPDATE brand_memberships SET status='revoked',updated_at=GREATEST(updated_at,now()) WHERE brand_id=$1 AND user_id=$2`, f.brand, editor)
	pvmCheck(t, err)
	pvmCheck(t, guard.Commit())
	if err = <-result; !errors.Is(err, usecase.ErrResourceHidden) {
		t.Fatal("revoked editor wrote through stale permissions", err)
	}
	variants, err := f.m.ProductVariants(ctx, f.owner, f.product, f.color)
	pvmCheck(t, err)
	if variants[0].Stock() != 1 {
		t.Fatal("revoked update committed")
	}
}
