package storage_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"os"
	"strconv"
	"testing"

	"srez/internal/entities"
	"srez/internal/objectstorage"
	"srez/internal/storage"
	"srez/internal/usecase"
)

type editorialStorageProbe struct {
	usecase.MediaObjects
	writes     map[string]int
	failDelete string
	failure    error
}

func (s *editorialStorageProbe) Put(ctx context.Context, key, mime string, body io.Reader) error {
	s.writes[key]++
	return s.MediaObjects.Put(ctx, key, mime, body)
}

func (s *editorialStorageProbe) Delete(ctx context.Context, key string) error {
	if s.failDelete == key {
		s.failDelete = ""
		return s.failure
	}
	return s.MediaObjects.Delete(ctx, key)
}

func TestPostgresEditorialMediaSnapshotsAndRetention(t *testing.T) {
	db := testDB(t)
	migrate(t, db)
	endpoint := os.Getenv("TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("TEST_S3_ENDPOINT is required")
	}
	bucket := "srez-media"
	objects, err := objectstorage.New(objectstorage.Config{
		Endpoint: endpoint, PublicEndpoint: endpoint, Region: "us-east-1", Bucket: bucket,
		AccessKey: os.Getenv("SREZ_S3_ACCESS_KEY"), SecretKey: os.Getenv("SREZ_S3_SECRET_KEY"),
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	users := usecase.NewUsers(storage.NewUsers(db))
	owner := managedUser(t, users, "editorial_owner")
	editor := managedUser(t, users, "editorial_editor")
	outsider := managedUser(t, users, "editorial_other")
	management := usecase.NewManagement(storage.NewManagement(db))
	repo := storage.NewMedia(db)
	probe := &editorialStorageProbe{MediaObjects: objects, writes: make(map[string]int), failure: errors.New("simulated temporary storage delete failure")}
	media := usecase.NewMedia(repo, probe, bucket, management)
	createBrand := func(id int64, slug string) int64 {
		t.Helper()
		b, e := management.CreateBrand(ctx, id, "Editorial Studio", slug)
		if e != nil {
			t.Fatal(e)
		}
		return b.ID()
	}
	brandID := createBrand(owner, "editorial-own")
	foreignID := createBrand(outsider, "editorial-other")
	var pngBytes bytes.Buffer
	if err = png.Encode(&pngBytes, image.NewGray(image.Rect(0, 0, 1600, 60))); err != nil {
		t.Fatal(err)
	}
	request := func(id int64, purpose string) usecase.MediaRequest {
		return usecase.MediaRequest{Purpose: purpose, BrandID: id, ContentType: "image/png", SizeBytes: int64(pngBytes.Len())}
	}
	upload := func(uid, id int64, purpose string) int64 {
		t.Helper()
		start, e := media.Create(ctx, uid, request(id, purpose))
		if e != nil {
			t.Fatal(e)
		}
		ready, e := media.Upload(ctx, uid, start.ID, "image/png", int64(pngBytes.Len()), bytes.NewReader(pngBytes.Bytes()))
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() {
			_ = objects.Delete(context.Background(), ready.Asset.ObjectKey)
			for _, width := range []int{480, 960, 1440} {
				_ = objects.Delete(context.Background(), ready.Asset.ObjectKey+".w"+strconv.Itoa(width)+"-v1")
			}
		})
		return ready.AssetID
	}
	upload(owner, brandID, "brand_logo")
	desc := "Original editorial studio."
	if _, err = management.ChangeBrand(ctx, owner, brandID, usecase.BrandPatch{Description: &desc}, "patch"); err != nil {
		t.Fatal(err)
	}
	if _, err = management.ChangeBrand(ctx, owner, brandID, usecase.BrandPatch{}, "publish"); err != nil {
		t.Fatal(err)
	}

	if _, err = management.Invite(ctx, owner, brandID, "editorial_editor"); err != nil {
		t.Fatal(err)
	}
	if _, err = management.Invitation(ctx, editor, brandID, true); err != nil {
		t.Fatal(err)
	}
	if _, err = media.Create(ctx, editor, request(brandID, usecase.EditorialMediaPurpose)); !errors.Is(err, usecase.ErrForbidden) {
		t.Fatal(err)
	}
	id := upload(owner, brandID, usecase.EditorialMediaPurpose)
	unused := upload(owner, brandID, usecase.EditorialMediaPurpose)
	foreign := upload(outsider, foreignID, usecase.EditorialMediaPurpose)
	pending, err := media.Create(ctx, owner, request(brandID, usecase.EditorialMediaPurpose))
	if err != nil {
		t.Fatal(err)
	}
	for _, uid := range []int64{0, editor, outsider} {
		if _, err = repo.ReadableAsset(ctx, uid, id); !errors.Is(err, usecase.ErrResourceHidden) {
			t.Fatal(uid, err)
		}
	}
	for _, uid := range []int64{0, editor, outsider} {
		if err = media.DeliverSize(ctx, uid, id, 480, func(*usecase.MediaAsset, []byte) { t.Error("private variant disclosed") }); !errors.Is(err, usecase.ErrResourceHidden) {
			t.Fatal(uid, err)
		}
	}
	if err = media.DeliverSize(ctx, owner, id, 480, func(*usecase.MediaAsset, []byte) {}); err != nil {
		t.Fatal(err)
	}
	if err = media.DeliverSize(ctx, 0, id, 480, func(*usecase.MediaAsset, []byte) { t.Error("cached private variant disclosed") }); !errors.Is(err, usecase.ErrResourceHidden) {
		t.Fatal(err)
	}
	if err = management.Revoke(ctx, owner, brandID, editor); err != nil {
		t.Fatal(err)
	}
	if _, err = media.Create(ctx, editor, request(brandID, usecase.EditorialMediaPurpose)); !errors.Is(err, usecase.ErrResourceHidden) {
		t.Fatal(err)
	}

	v1 := entities.DefaultBrandPage(entities.BrandAppearance{})
	if _, err = management.SaveBrandPage(ctx, owner, brandID, 0, v1); err != nil {
		t.Fatal(err)
	}
	if _, err = management.PublishBrandPage(ctx, owner, brandID, 1); err != nil {
		t.Fatal(err)
	}
	photo := func(blockID string, aid int64) entities.BrandPageBlock {
		return entities.BrandPageBlock{ID: blockID, Type: entities.BrandPageImageBlock, Layout: "contained",
			Content: entities.BrandPageContent{Source: "editorial", Images: []entities.BrandPageImage{{MediaID: aid}}}}
	}
	v3 := entities.DefaultBrandPage(entities.BrandAppearance{})
	v3.SchemaVersion = 3
	v3.Blocks[1].Layout = "editorial-reversed"
	v3.Blocks[1].Content.Images = []entities.BrandPageImage{{MediaID: foreign}}
	if _, err = management.SaveBrandPage(ctx, owner, brandID, 2, v3); !errors.Is(err, entities.ErrInvalidValue) {
		t.Fatalf("accepted foreign story asset: %v", err)
	}

	v2 := v1
	v2.SchemaVersion = 2
	for _, unusable := range []int64{foreign, pending.AssetID, 999999} {
		v2.Blocks = append(v1.Blocks, photo("image-one", id), photo("image-two", unusable))
		if _, err = management.SaveBrandPage(ctx, owner, brandID, 2, v2); !errors.Is(err, entities.ErrInvalidValue) {
			t.Fatalf("accepted unavailable asset: %v", err)
		}
	}
	v2.Blocks = append(v1.Blocks, photo("image-one", id), photo("image-two", id))
	state, err := management.EditBrandPage(ctx, owner, brandID)
	if err != nil || state.Revision != 2 || state.Draft.SchemaVersion != 1 || state.Published.SchemaVersion != 1 || len(state.MediaIDs) != 2 {
		t.Fatal(state, err)
	}
	if _, err = management.SaveBrandPage(ctx, owner, brandID, 2, v2); err != nil {
		t.Fatal(err)
	}
	assertPublicRule := func(revision int, readable bool) {
		t.Helper()
		b, e := storage.NewBrands(db).GetActiveBySlug(ctx, "editorial-own")
		if e != nil || b == nil || b.PublishedPage().SchemaVersion != revision {
			t.Fatal(b, e)
		}
		_, e = repo.ReadableAsset(ctx, 0, id)
		if readable && e != nil || !readable && !errors.Is(e, usecase.ErrResourceHidden) {
			t.Fatal(e)
		}
	}
	assertPublicRule(1, false)

	races := make(chan error, 2)
	go func() { _, e := management.PublishBrandPage(ctx, owner, brandID, 3); races <- e }()
	go func() { _, e := management.SaveBrandPage(ctx, owner, brandID, 3, v2); races <- e }()
	first, second := <-races, <-races
	if !(first == nil && errors.Is(second, usecase.ErrConflict) || second == nil && errors.Is(first, usecase.ErrConflict)) {
		t.Fatal(first, second)
	}
	state, err = management.EditBrandPage(ctx, owner, brandID)
	if err != nil || state.Revision != 4 {
		t.Fatal(state, err)
	}
	if state.Published.SchemaVersion == 1 {
		state, err = management.PublishBrandPage(ctx, owner, brandID, state.Revision)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err = media.DeliverSize(ctx, 0, id, 480, func(_ *usecase.MediaAsset, data []byte) {
		if len(data) == 0 {
			t.Error("empty variant")
		}
	}); err != nil {
		t.Fatal(err)
	}
	ready, err := repo.ReadableAsset(ctx, 0, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{480, 960, 1440} {
		var first []byte
		for attempt := 0; attempt < 2; attempt++ {
			if err := media.DeliverSize(ctx, 0, id, width, func(_ *usecase.MediaAsset, data []byte) {
				cfg, _, decodeErr := image.DecodeConfig(bytes.NewReader(data))
				if decodeErr != nil || cfg.Width != width {
					t.Errorf("variant dimensions: %v %v", cfg, decodeErr)
				}
				if attempt == 0 {
					first = data
				} else if !bytes.Equal(data, first) {
					t.Error("repeated delivery changed cached variant")
				}
			}); err != nil {
				t.Fatal(err)
			}
		}
		key := ready.ObjectKey + ".w" + strconv.Itoa(width) + "-v1"
		if probe.writes[key] != 1 {
			t.Fatalf("expected one variant creation at width %d", width)
		}
	}
	assertPublicRule(2, true)
	if _, data, e := media.Read(ctx, 0, id); e != nil || len(data) == 0 {
		t.Fatal(e)
	}

	state, err = management.SaveBrandPage(ctx, owner, brandID, state.Revision, v1)
	if err != nil {
		t.Fatal(err)
	}
	assertPublicRule(2, true)
	if _, err = db.Exec(`UPDATE brand_editorial_media SET retain_until=now()-interval '1 day' WHERE brand_id=$1`, brandID); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Garbage(ctx); err != nil {
		t.Fatal(err)
	}
	assertPublicRule(2, true)
	if _, err = repo.ReadableAsset(ctx, owner, unused); !errors.Is(err, usecase.ErrResourceHidden) {
		t.Fatal(err)
	}
	if _, err = management.PublishBrandPage(ctx, owner, brandID, state.Revision); err != nil {
		t.Fatal(err)
	}
	a, err := repo.ReadableAsset(ctx, owner, id)
	if err != nil {
		t.Fatal(err)
	}
	if err = media.DeliverSize(ctx, 0, id, 480, func(*usecase.MediaAsset, []byte) { t.Error("retired public variant disclosed") }); !errors.Is(err, usecase.ErrResourceHidden) {
		t.Fatal(err)
	}
	assertPublicRule(1, false)
	var retained bool
	if err = db.Get(&retained, `SELECT retain_until>now() FROM brand_editorial_media WHERE asset_id=$1`, id); err != nil || !retained {
		t.Fatal(retained, err)
	}
	if _, err = db.Exec(`UPDATE brand_editorial_media SET retain_until=now()-interval '1 day' WHERE asset_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Garbage(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE media_assets SET delete_after=now()-interval '1 minute' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	probe.failDelete = a.ObjectKey + ".w960-v1"
	if err = media.Cleanup(ctx); !errors.Is(err, probe.failure) {
		t.Fatal("expected transient delete failure", err)
	}
	var status string
	if err = db.Get(&status, `SELECT state FROM media_assets WHERE id=$1`, id); err != nil || status != "deleting" {
		t.Fatal(status, err)
	}
	for _, key := range []string{a.ObjectKey, a.ObjectKey + ".w960-v1"} {
		object, err := objects.Get(ctx, key)
		if err != nil {
			t.Fatal("retryable cleanup removed the remaining object", err)
		}
		object.Close()
	}
	if err = media.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", ".w480-v1", ".w960-v1", ".w1440-v1"} {
		object, err := objects.Get(ctx, a.ObjectKey+suffix)
		if object != nil {
			object.Close()
		}
		if !errors.Is(err, usecase.ErrMediaObjectMissing) {
			t.Fatalf("object with suffix %q survived GC: %v", suffix, err)
		}
	}
	if err = db.Get(&status, `SELECT state FROM media_assets WHERE id=$1`, id); err != nil || status != "deleted" {
		t.Fatal(status, err)
	}
}
