package storage_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"srez/internal/entities"
	"srez/internal/storage"
	"srez/internal/usecase"
)

func TestPostgresBrandPageRevisionAndMembership(t *testing.T) {
	db := testDB(t)
	migrate(t, db)
	ctx := context.Background()
	users := usecase.NewUsers(storage.NewUsers(db))
	owner := managedUser(t, users, "page_owner")
	editor := managedUser(t, users, "page_editor")
	other := managedUser(t, users, "page_other")
	service := usecase.NewManagement(storage.NewManagement(db))
	brand, err := service.CreateBrand(ctx, owner, "Editorial brand", "page-brand")
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := service.CreateBrand(ctx, other, "Foreign brand", "page-foreign")
	if err != nil {
		t.Fatal(err)
	}
	id := brand.ID()
	if _, err = service.EditBrandPage(ctx, owner, foreign.ID()); !errors.Is(err, usecase.ErrResourceHidden) {
		t.Fatalf("cross-brand read: %v", err)
	}
	if _, err = service.Invite(ctx, owner, id, "page_editor"); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Invitation(ctx, editor, id, true); err != nil {
		t.Fatal(err)
	}
	initial, err := service.EditBrandPage(ctx, owner, id)
	if err != nil || initial.Revision != 0 || initial.Published != nil {
		t.Fatal(initial, err)
	}
	if _, err = service.SaveBrandPage(ctx, editor, id, 0, initial.Draft); !errors.Is(err, usecase.ErrForbidden) {
		t.Fatalf("editor write: %v", err)
	}
	if _, err = service.PublishBrandPage(ctx, other, id, 0); !errors.Is(err, usecase.ErrResourceHidden) {
		t.Fatalf("outsider publish: %v", err)
	}
	if err = service.Revoke(ctx, owner, id, editor); err != nil {
		t.Fatal(err)
	}
	if _, err = service.EditBrandPage(ctx, editor, id); !errors.Is(err, usecase.ErrResourceHidden) {
		t.Fatalf("revoked read: %v", err)
	}
	var writers sync.WaitGroup
	results := make(chan error, 2)
	for _, color := range []string{"#121212", "#ababab"} {
		writers.Go(func() {
			config := entities.DefaultBrandPage(entities.BrandAppearance{AccentColor: color})
			_, err := service.SaveBrandPage(ctx, owner, id, 0, config)
			results <- err
		})
	}
	writers.Wait()
	close(results)
	success, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, usecase.ErrConflict):
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatal(success, conflicts)
	}
	saved, err := service.EditBrandPage(ctx, owner, id)
	if err != nil || saved.Revision != 1 || saved.Draft.Appearance.AccentColor == "" || saved.Published != nil {
		t.Fatal(saved, err)
	}
	published, err := service.PublishBrandPage(ctx, owner, id, saved.Revision)
	if err != nil || published.Revision != 2 || published.PublishedRevision == nil || *published.PublishedRevision != 2 || published.Published == nil {
		t.Fatal(published, err)
	}
	if b, err := storage.NewBrands(db).GetActiveBySlug(ctx, "page-brand"); err != nil || b != nil {
		t.Fatal("published layout must not publish the brand", b, err)
	}
	for _, version := range []int{2, 3} {
		candidate := entities.DefaultBrandPage(entities.BrandAppearance{})
		candidate.SchemaVersion = version
		draft, err := service.SaveBrandPage(ctx, owner, id, published.Revision, candidate)
		if err != nil {
			t.Fatal(err)
		}
		published, err = service.PublishBrandPage(ctx, owner, id, draft.Revision)
		if err != nil || published.Published.SchemaVersion != version {
			t.Fatal(err)
		}
	}
	reader := usecase.NewManagement(storage.NewManagement(db))
	if err = reader.LimitBrandPageWrites(2); err != nil {
		t.Fatal(err)
	}
	if read, err := reader.EditBrandPage(ctx, owner, id); err != nil || read.Published.SchemaVersion != 3 {
		t.Fatal(err)
	}
	if _, err = reader.SaveBrandPage(ctx, owner, id, published.Revision, published.Draft); !errors.Is(err, usecase.ErrBrandPageWritesLimited) {
		t.Fatal(err)
	}
	if _, err = reader.PublishBrandPage(ctx, owner, id, published.Revision); !errors.Is(err, usecase.ErrBrandPageWritesLimited) {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE brands SET status='suspended' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err = service.EditBrandPage(ctx, owner, id); !errors.Is(err, usecase.ErrForbidden) {
		t.Fatal(err)
	}
}
