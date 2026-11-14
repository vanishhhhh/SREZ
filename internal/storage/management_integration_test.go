package storage_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/lib/pq"
	"os"
	"srez/migrations"
	"sync"
	"testing"
	"time"

	"srez/internal/entities"
	"srez/internal/storage"
	"srez/internal/usecase"
)

func managedUser(t *testing.T, users *usecase.Users, name string) int64 {
	t.Helper()
	u, e := users.Create(context.Background(), name+"@example.com", name, name)
	if e != nil {
		t.Fatal(e)
	}
	return u.ID()
}
func draftProduct(t *testing.T, brand int64) *entities.Product {
	t.Helper()
	p, e := entities.NewProduct(brand, "Draft product", "", 0, "", nil, time.Now().UTC())
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func TestPostgresManagementPolicy(t *testing.T) {
	db := testDB(t)
	migrate(t, db)
	seed(t, db)
	ctx := context.Background()
	users := usecase.NewUsers(storage.NewUsers(db))
	owner := managedUser(t, users, "owner")
	editor := managedUser(t, users, "editor")
	outsider := managedUser(t, users, "outsider")
	m := usecase.NewManagement(storage.NewManagement(db))
	if _, e := m.GetBrand(ctx, owner, 1); !errors.Is(e, usecase.ErrResourceHidden) {
		t.Fatal("legacy brand claim", e)
	}
	if _, e := m.Invite(ctx, owner, 1, "owner"); !errors.Is(e, usecase.ErrResourceHidden) {
		t.Fatal(e)
	}
	b, e := m.CreateBrand(ctx, owner, "Own brand", "own-brand")
	if e != nil {
		t.Fatal(e)
	}
	if b.Status() != entities.BrandDraft {
		t.Fatal("not draft")
	}
	members, e := m.Members(ctx, owner, b.ID())
	if e != nil || len(members) != 1 || members[0].Role() != entities.BrandOwner {
		t.Fatal(members, e)
	}
	if _, e = m.CreateBrand(ctx, outsider, "Duplicate", "own-brand"); !errors.Is(e, usecase.ErrConflict) {
		t.Fatal(e)
	}
	if _, e = m.CreateBrand(ctx, 0, "Invalid", "invalid-brand"); !errors.Is(e, usecase.ErrForbidden) {
		t.Fatal(e)
	}
	before := count(t, db, "brands")
	if _, e = m.CreateBrand(ctx, 9223372036854775807, "Rollback", "rollback-brand"); e == nil {
		t.Fatal("missing owner accepted")
	}
	if count(t, db, "brands") != before {
		t.Fatal("orphan brand on membership failure")
	}
	if _, e = m.ChangeBrand(ctx, owner, b.ID(), usecase.BrandPatch{}, "publish"); !errors.Is(e, entities.ErrNotReady) {
		t.Fatal(e)
	}
	if _, e = m.Invite(ctx, owner, b.ID(), "owner"); !errors.Is(e, usecase.ErrConflict) {
		t.Fatal(e)
	}
	invitation, e := m.Invite(ctx, owner, b.ID(), "editor")
	if e != nil || invitation.Status() != entities.MembershipInvited {
		t.Fatal(e)
	}
	mine, e := m.MyBrands(ctx, editor, true)
	if e != nil || len(mine) != 1 {
		t.Fatal(mine, e)
	}
	if _, e = m.CreateProduct(ctx, editor, *draftProduct(t, b.ID())); !errors.Is(e, usecase.ErrResourceHidden) {
		t.Fatal("invited has permissions", e)
	}
	if _, e = m.Invitation(ctx, outsider, b.ID(), true); !errors.Is(e, usecase.ErrResourceHidden) {
		t.Fatal("accepted another invitation", e)
	}
	if _, e = m.Invitation(ctx, editor, b.ID(), false); e != nil {
		t.Fatal(e)
	}
	if _, e = m.GetBrand(ctx, editor, b.ID()); !errors.Is(e, usecase.ErrResourceHidden) {
		t.Fatal(e)
	}
	if _, e = m.Invite(ctx, owner, b.ID(), "editor"); e != nil {
		t.Fatal(e)
	}
	if _, e = m.Invitation(ctx, editor, b.ID(), true); e != nil {
		t.Fatal(e)
	}
	if _, e = m.Invite(ctx, owner, b.ID(), "editor"); !errors.Is(e, entities.ErrInvalidTransition) {
		t.Fatal(e)
	}
	if _, e = m.Invite(ctx, editor, b.ID(), "outsider"); !errors.Is(e, usecase.ErrForbidden) {
		t.Fatal(e)
	}
	name := "Forbidden"
	if _, e = m.ChangeBrand(ctx, editor, b.ID(), usecase.BrandPatch{Name: &name}, "patch"); !errors.Is(e, usecase.ErrForbidden) {
		t.Fatal(e)
	}
	other, e := m.CreateBrand(ctx, outsider, "Other brand", "other-brand")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = m.ChangeBrand(ctx, owner, other.ID(), usecase.BrandPatch{Name: &name}, "patch"); !errors.Is(e, usecase.ErrResourceHidden) {
		t.Fatal("BOLA", e)
	}
	p, e := m.CreateProduct(ctx, editor, *draftProduct(t, b.ID()))
	if e != nil {
		t.Fatal(e)
	}
	otherProduct, err := m.CreateProduct(ctx, outsider, *draftProduct(t, other.ID()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.ChangeProduct(ctx, editor, otherProduct.ID(), usecase.ProductPatch{Name: &name}, "patch"); !errors.Is(err, usecase.ErrResourceHidden) {
		t.Fatal("editor cross-brand BOLA", err)
	}
	if _, e = m.ChangeProduct(ctx, outsider, p.ID(), usecase.ProductPatch{Name: &name}, "patch"); !errors.Is(e, usecase.ErrResourceHidden) {
		t.Fatal("product BOLA", e)
	}
	if _, e = m.ChangeProduct(ctx, editor, p.ID(), usecase.ProductPatch{}, "publish"); !errors.Is(e, usecase.ErrConflict) {
		t.Fatal("draft brand publication", e)
	}
	desc, logo := "Brand description", "https://example.com/logo.png"
	if _, e = m.ChangeBrand(ctx, owner, b.ID(), usecase.BrandPatch{Description: &desc, LogoURL: &logo}, "patch"); e != nil {
		t.Fatal(e)
	}
	if _, e = m.ChangeBrand(ctx, owner, b.ID(), usecase.BrandPatch{}, "publish"); e != nil {
		t.Fatal(e)
	}
	if _, e = m.ChangeProduct(ctx, editor, p.ID(), usecase.ProductPatch{}, "publish"); !errors.Is(e, entities.ErrNotReady) {
		t.Fatal("incomplete product published", e)
	}
	public := usecase.NewProducts(storage.NewProducts(db), storage.NewBrands(db))
	if _, e = public.Get(ctx, p.ID()); !errors.Is(e, usecase.ErrProductNotFound) {
		t.Fatal("draft exposed", e)
	}
	if e = m.Revoke(ctx, owner, b.ID(), owner); !errors.Is(e, usecase.ErrForbidden) {
		t.Fatal("owner revoked", e)
	}
	if e = m.Revoke(ctx, owner, b.ID(), editor); e != nil {
		t.Fatal(e)
	}
	if _, e = m.ChangeProduct(ctx, editor, p.ID(), usecase.ProductPatch{Name: &name}, "patch"); !errors.Is(e, usecase.ErrResourceHidden) {
		t.Fatal("revoked permission", e)
	}
	if _, e = m.Invite(ctx, owner, b.ID(), "editor"); e != nil {
		t.Fatal(e)
	}
	if _, e = m.CreateProduct(ctx, editor, *draftProduct(t, b.ID())); !errors.Is(e, usecase.ErrResourceHidden) {
		t.Fatal("reinvite skips accept", e)
	}
	if _, e = m.Invitation(ctx, editor, b.ID(), true); e != nil {
		t.Fatal(e)
	}
	exec(t, db, `UPDATE brands SET status='suspended' WHERE id=$1`, b.ID())
	if _, e = m.CreateProduct(ctx, editor, *draftProduct(t, b.ID())); !errors.Is(e, usecase.ErrForbidden) {
		t.Fatal(e)
	}
	if _, e = m.ChangeProduct(ctx, owner, p.ID(), usecase.ProductPatch{Name: &name}, "patch"); !errors.Is(e, usecase.ErrForbidden) {
		t.Fatal(e)
	}
	if _, e = m.Products(ctx, owner, b.ID()); !errors.Is(e, usecase.ErrForbidden) {
		t.Fatal(e)
	}
}
func TestPostgresManagementFullAggregate(t *testing.T) {
	db := testDB(t)
	migrate(t, db)
	seed(t, db)
	ctx := context.Background()
	users := usecase.NewUsers(storage.NewUsers(db))
	owner := managedUser(t, users, "aggregate_owner")
	m := usecase.NewManagement(storage.NewManagement(db))
	// Trusted test-only operator assignment, never an HTTP claim endpoint.
	exec(t, db, `INSERT INTO brand_memberships(user_id,brand_id,role,status,joined_at,created_at,updated_at) VALUES($1,1,'owner','active',now(),now(),now())`, owner)
	if _, e := m.ChangeProduct(ctx, owner, 1, usecase.ProductPatch{}, "hide"); e != nil {
		t.Fatal(e)
	}
	if _, e := m.ChangeProduct(ctx, owner, 1, usecase.ProductPatch{}, "publish"); e != nil {
		t.Fatal(e)
	}
	if _, e := m.ChangeProduct(ctx, owner, 1, usecase.ProductPatch{}, "unpublish"); e != nil {
		t.Fatal(e)
	}
	if _, e := m.ChangeProduct(ctx, owner, 1, usecase.ProductPatch{}, "publish"); e != nil {
		t.Fatal(e)
	}
	// A second active owner is rejected by PostgreSQL itself.
	other := managedUser(t, users, "another_owner")
	if _, e := db.Exec(`INSERT INTO brand_memberships(user_id,brand_id,role,status,joined_at,created_at,updated_at) VALUES($1,1,'owner','active',now(),now(),now())`, other); e == nil {
		t.Fatal("two owners")
	}
}
func TestPostgresManagementConcurrency(t *testing.T) {
	db := testDB(t)
	migrate(t, db)
	ctx := context.Background()
	users := usecase.NewUsers(storage.NewUsers(db))
	owner := managedUser(t, users, "concurrent_owner")
	editor := managedUser(t, users, "concurrent_editor")
	m := usecase.NewManagement(storage.NewManagement(db))
	b, e := m.CreateBrand(ctx, owner, "Concurrent", "concurrent")
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := m.Invite(ctx, owner, b.ID(), "concurrent_editor"); errs <- e }()
	}
	wg.Wait()
	close(errs)
	success := 0
	for e := range errs {
		if e == nil {
			success++
		} else if !errors.Is(e, entities.ErrInvalidTransition) {
			t.Fatal(e)
		}
	}
	if success != 1 {
		t.Fatal("duplicate invitation", success)
	}
	// Accept vs revoke can linearize either way; final membership is revoked.
	errs = make(chan error, 2)
	wg.Add(2)
	go func() { defer wg.Done(); _, e := m.Invitation(ctx, editor, b.ID(), true); errs <- e }()
	go func() { defer wg.Done(); errs <- m.Revoke(ctx, owner, b.ID(), editor) }()
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil && !errors.Is(e, entities.ErrInvalidTransition) {
			t.Fatal(e)
		}
	}
	if _, e = m.GetBrand(ctx, editor, b.ID()); !errors.Is(e, usecase.ErrResourceHidden) {
		t.Fatal("revocation lost", e)
	}
	if _, e = m.Invite(ctx, owner, b.ID(), "concurrent_editor"); e != nil {
		t.Fatal(e)
	}
	if _, e = m.Invitation(ctx, editor, b.ID(), true); e != nil {
		t.Fatal(e)
	}
	p, e := m.CreateProduct(ctx, owner, *draftProduct(t, b.ID()))
	if e != nil {
		t.Fatal(e)
	}
	// Deterministically hold the brand lock, revoke, then release a waiting edit.
	tx, e := db.BeginTxx(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	var id int64
	if e = tx.GetContext(ctx, &id, `SELECT id FROM brands WHERE id=$1 FOR UPDATE`, b.ID()); e != nil {
		t.Fatal(e)
	}
	started := make(chan struct{})
	done := make(chan error, 1)
	name := "Must not persist"
	go func() {
		close(started)
		_, e := m.ChangeProduct(ctx, editor, p.ID(), usecase.ProductPatch{Name: &name}, "patch")
		done <- e
	}()
	<-started
	// PostgreSQL may run in a VM whose clock lags Go on the host. Preserve the
	// membership's existing chronology in this direct-SQL test fixture.
	if _, e = tx.ExecContext(ctx, `UPDATE brand_memberships SET status='revoked',updated_at=GREATEST(updated_at,now()) WHERE brand_id=$1 AND user_id=$2`, b.ID(), editor); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	if e = <-done; !errors.Is(e, usecase.ErrResourceHidden) {
		t.Fatal("TOCTOU edit", e)
	}
	var saved string
	if e = db.Get(&saved, `SELECT name FROM products WHERE id=$1`, p.ID()); e != nil || saved == name {
		t.Fatal("unauthorized write committed", e)
	}
}

func TestPostgresParallelPublicationAndOwnerCreation(t *testing.T) {
	db := testDB(t)
	migrate(t, db)
	seed(t, db)
	ctx := context.Background()
	users := usecase.NewUsers(storage.NewUsers(db))
	owner := managedUser(t, users, "parallel_owner")
	other := managedUser(t, users, "parallel_other")
	m := usecase.NewManagement(storage.NewManagement(db))
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, uid := range []int64{owner, other} {
		wg.Add(1)
		go func(uid int64) {
			defer wg.Done()
			_, e := m.CreateBrand(ctx, uid, "Race brand", "race-brand")
			errs <- e
		}(uid)
	}
	wg.Wait()
	close(errs)
	success := 0
	for e := range errs {
		if e == nil {
			success++
		} else if !errors.Is(e, usecase.ErrConflict) {
			t.Fatal(e)
		}
	}
	if success != 1 || count(t, db, "brand_memberships") != 1 {
		t.Fatal("owner race", success)
	}
	exec(t, db, `INSERT INTO brand_memberships(user_id,brand_id,role,status,joined_at,created_at,updated_at) VALUES($1,1,'owner','active',now(),now(),now())`, owner)
	if _, e := m.ChangeProduct(ctx, owner, 1, usecase.ProductPatch{}, "hide"); e != nil {
		t.Fatal(e)
	}
	errs = make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := m.ChangeProduct(ctx, owner, 1, usecase.ProductPatch{}, "publish")
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	success = 0
	for e := range errs {
		if e == nil {
			success++
		} else if !errors.Is(e, entities.ErrInvalidTransition) {
			t.Fatal(e)
		}
	}
	if success != 1 {
		t.Fatal("publication race", success)
	}
}

func TestPostgresUpgradeFromAuthWithLegacyBrands(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	exec(t, db, `CREATE TABLE schema_migrations(version TEXT PRIMARY KEY,checksum TEXT NOT NULL,applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`)
	for _, name := range []string{"001_users.sql", "002_catalog.sql", "003_auth.sql"} {
		body, e := os.ReadFile("../../migrations/" + name)
		if e != nil {
			t.Fatal(e)
		}
		exec(t, db, string(body))
		exec(t, db, `INSERT INTO schema_migrations(version,checksum) VALUES($1,$2)`, name, fmt.Sprintf("%x", sha256.Sum256(body)))
	}
	seed(t, db)
	beforeBrands, beforeProducts := count(t, db, "brands"), count(t, db, "products")
	if e := migrations.Apply(ctx, db); e != nil {
		t.Fatal(e)
	}
	if count(t, db, "brands") != beforeBrands || count(t, db, "products") != beforeProducts || count(t, db, "brand_memberships") != 0 {
		t.Fatal("legacy backfill changed data")
	}
}

func TestPostgresPublicationLocksRelatedWrites(t *testing.T) {
	db := testDB(t)
	migrate(t, db)
	seed(t, db)
	ctx := context.Background()
	repo := storage.NewManagement(db)
	e := repo.WithinBrand(ctx, 1, func(tx usecase.ManagementTx, _ *entities.Brand) error {
		if _, e := tx.Product(ctx, 1); e != nil {
			return e
		}
		if _, e := tx.Publication(ctx, 1); e != nil {
			return e
		}
		for _, query := range []string{
			`UPDATE product_colors SET color=color WHERE product_id=1`,
			`UPDATE product_color_images SET url=url WHERE product_color_id IN (SELECT id FROM product_colors WHERE product_id=1)`,
			`UPDATE product_variants SET stock=stock WHERE product_color_id IN (SELECT id FROM product_colors WHERE product_id=1)`,
			`INSERT INTO product_colors(product_id,color,color_key) VALUES(1,'Probe','probe')`,
		} {
			writer, e := db.BeginTxx(ctx, nil)
			if e != nil {
				return e
			}
			if _, e = writer.ExecContext(ctx, `SET LOCAL lock_timeout='200ms'`); e != nil {
				writer.Rollback()
				return e
			}
			_, e = writer.ExecContext(ctx, query)
			writer.Rollback()
			var pg *pq.Error
			if !errors.As(e, &pg) || pg.Code != "55P03" {
				return fmt.Errorf("related write was not blocked by aggregate locks: %v", e)
			}
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
}
