package usecase

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"testing"
	"time"

	"srez/internal/entities"
)

func jpegFixture(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	_ = jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 8, 8)), nil)
	return b.Bytes()
}
func TestNormalizeMedia(t *testing.T) {
	jpegData := jpegFixture(t)
	metadata := []byte("Exif-sensitive-location")
	segment := append([]byte{0xff, 0xe1, 0, byte(len(metadata) + 2)}, metadata...)
	jpegData = append(append(append([]byte{}, jpegData[:2]...), segment...), jpegData[2:]...)
	out, mime, e := NormalizeImage(jpegData, "image/jpeg")
	if e != nil || mime != "image/jpeg" || bytes.Contains(out, metadata) {
		t.Fatal("JPEG normalization", e)
	}
	if _, e = jpeg.Decode(bytes.NewReader(out)); e != nil {
		t.Fatal(e)
	}
	var b bytes.Buffer
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	img.Set(0, 0, color.NRGBA{R: 255, A: 128})
	_ = png.Encode(&b, img)
	out, mime, e = NormalizeImage(b.Bytes(), "image/png")
	if e != nil || mime != "image/png" {
		t.Fatal("alpha not preserved", e)
	}
	if _, e = png.Decode(bytes.NewReader(out)); e != nil {
		t.Fatal(e)
	}
	webpData, _ := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA")
	if _, mime, e = NormalizeImage(webpData, "image/webp"); e != nil || mime != "image/jpeg" {
		t.Fatal("WebP", e)
	}
	for _, v := range []struct {
		data []byte
		mime string
	}{{[]byte("<svg/>"), "image/jpeg"}, {jpegData, "image/png"}, {jpegData[:len(jpegData)/2], "image/jpeg"}, {nil, "image/webp"}} {
		if _, _, e = NormalizeImage(v.data, v.mime); !errors.Is(e, entities.ErrInvalidValue) {
			t.Fatal("unsafe source", e)
		}
	}
	if _, _, e = NormalizeImage(make([]byte, MaxMediaBytes+1), "image/jpeg"); !errors.Is(e, ErrMediaTooLarge) {
		t.Fatal("source limit")
	}
	for _, r := range []image.Rectangle{image.Rect(0, 0, 8193, 1), image.Rect(0, 0, 4000, 4000)} {
		b.Reset()
		_ = png.Encode(&b, image.NewGray(r))
		if _, _, e = NormalizeImage(b.Bytes(), "image/png"); e == nil {
			t.Fatal("pixel/dimension bomb")
		}
	}
	var limited boundedImageBuffer
	if _, e = limited.Write(make([]byte, MaxMediaOutputBytes)); e != nil {
		t.Fatal(e)
	}
	if _, e = limited.Write([]byte{1}); !errors.Is(e, ErrMediaTooLarge) {
		t.Fatal("output limit")
	}
}

type mediaRepoFake struct {
	u          MediaUpload
	user       *entities.User
	failCommit bool
	ambiguous  bool
	rejects    int
}

func (r *mediaRepoFake) WithinMediaUser(ctx context.Context, id int64, fn func(MediaTx, *entities.User) error) error {
	if id != 7 {
		return ErrResourceHidden
	}
	savedUpload := r.u
	savedUser := *r.user
	v := *r.user
	e := fn(r, &v)
	if e == nil && r.failCommit {
		e = errors.New("database commit failed")
	}
	if e != nil && !r.ambiguous {
		r.u = savedUpload
		*r.user = savedUser
	}
	return e
}
func (r *mediaRepoFake) CreateMediaUpload(_ context.Context, u *MediaUpload) error {
	u.ID = 1
	u.AssetID = 1
	u.State = "pending"
	u.Asset.ID = 1
	u.Asset.State = "processing"
	u.Asset.ExpiresAt = time.Now().Add(time.Minute)
	r.u = *u
	return nil
}
func (r *mediaRepoFake) LockMediaUpload(ctx context.Context, uid, id int64) (*MediaUpload, error) {
	return r.Upload(ctx, uid, id)
}
func (r *mediaRepoFake) ClaimMediaUpload(context.Context, int64) error {
	if r.u.State != "pending" {
		return ErrConflict
	}
	r.u.State = "processing"
	return nil
}
func (r *mediaRepoFake) ReadyMediaUpload(_ context.Context, u *MediaUpload) error {
	u.State = "complete"
	u.Asset.State = "ready"
	r.u = *u
	return nil
}
func (r *mediaRepoFake) SaveMediaUser(_ context.Context, u entities.User) error {
	*r.user = u
	return nil
}
func (r *mediaRepoFake) SaveMediaBrand(context.Context, entities.Brand) error { return ErrConflict }
func (r *mediaRepoFake) Upload(_ context.Context, uid, id int64) (*MediaUpload, error) {
	if uid != 7 || id != r.u.ID {
		return nil, ErrResourceHidden
	}
	v := r.u
	return &v, nil
}
func (r *mediaRepoFake) RejectUpload(context.Context, int64) error {
	r.rejects++
	r.u.State = "rejected"
	return nil
}
func (r *mediaRepoFake) ReadableAsset(_ context.Context, uid, id int64) (*MediaAsset, error) {
	if uid != 7 || id != r.u.AssetID || r.u.Asset.State != "ready" || r.user.AvatarMediaID() != id {
		return nil, ErrResourceHidden
	}
	return &r.u.Asset, nil
}
func (r *mediaRepoFake) Garbage(context.Context) ([]MediaAsset, error) {
	if r.u.Asset.State == "processing" {
		return []MediaAsset{r.u.Asset}, nil
	}
	return nil, nil
}
func (r *mediaRepoFake) Deleted(context.Context, int64) error {
	r.u.Asset.State = "deleted"
	return nil
}
func (r *mediaRepoFake) CleanupLegacy(context.Context, MediaObjects, string) error { return nil }

type mediaObjectsFake struct {
	stored   map[string][]byte
	putCount int
	putErr   error
	afterPut func()
}

func (o *mediaObjectsFake) Ready(context.Context) error { return nil }
func (o *mediaObjectsFake) Get(_ context.Context, key string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(o.stored[key])), nil
}
func (o *mediaObjectsFake) Put(_ context.Context, k, m string, b io.Reader) error {
	o.putCount++
	if o.putErr != nil {
		return o.putErr
	}
	v, e := io.ReadAll(b)
	o.stored[k] = v
	if o.afterPut != nil {
		o.afterPut()
	}
	return e
}
func (o *mediaObjectsFake) Delete(_ context.Context, k string) error { delete(o.stored, k); return nil }
func fixtureMedia(t *testing.T) (*Media, *mediaRepoFake, *mediaObjectsFake, []byte) {
	t.Helper()
	u, e := entities.NewUser("media@example.com", "mediauser", "Media", "!", time.Now().Add(-time.Minute))
	if e != nil {
		t.Fatal(e)
	}
	state := u.State()
	state.ID = 7
	u, e = entities.RestoreUser(state)
	if e != nil {
		t.Fatal(e)
	}
	r := &mediaRepoFake{user: u}
	o := &mediaObjectsFake{stored: map[string][]byte{}}
	m := NewMedia(r, o, "test")
	data := jpegFixture(t)
	_, e = m.Create(context.Background(), 7, MediaRequest{Purpose: "user_avatar", ContentType: "image/jpeg", SizeBytes: int64(len(data))})
	if e != nil {
		t.Fatal(e)
	}
	return m, r, o, data
}
func TestMediaPreS3LimitsAndValidation(t *testing.T) {
	for _, kind := range []string{"length", "actual", "missing", "short", "mime", "corrupt", "read-limit"} {
		t.Run(kind, func(t *testing.T) {
			m, r, o, data := fixtureMedia(t)
			length := int64(len(data))
			mime := "image/jpeg"
			var body io.Reader = bytes.NewReader(data)
			switch kind {
			case "length":
				length = MaxMediaBytes + 1
			case "actual":
				body = bytes.NewReader(make([]byte, MaxMediaBytes+1))
			case "missing":
				length = -1
			case "short":
				body = bytes.NewReader(data[:10])
			case "mime":
				mime = "text/plain"
			case "corrupt":
				body = bytes.NewReader(bytes.Repeat([]byte("x"), len(data)))
			case "read-limit":
				body = http.MaxBytesReader(nil, io.NopCloser(bytes.NewReader(make([]byte, MaxMediaBytes+1))), MaxMediaBytes)
			}
			_, e := m.Upload(context.Background(), 7, r.u.ID, mime, length, body)
			if e == nil || o.putCount != 0 || len(o.stored) != 0 {
				t.Fatal("rejected source reached S3", e)
			}
		})
	}
}
func TestMediaAvatarAtomicityRetryAndCleanup(t *testing.T) {
	m, r, o, data := fixtureMedia(t)
	ctx := context.Background()
	if _, e := m.Upload(ctx, 8, 1, "image/jpeg", int64(len(data)), bytes.NewReader(data)); !errors.Is(e, ErrResourceHidden) {
		t.Fatal("foreign upload", e)
	}
	u, e := m.Upload(ctx, 7, 1, "image/jpeg", int64(len(data)), bytes.NewReader(data))
	if e != nil || u.State != "complete" || r.user.AvatarMediaID() != u.AssetID {
		t.Fatal("unbound upload", e)
	}
	if _, e = m.Upload(ctx, 7, 1, "image/jpeg", int64(len(data)), bytes.NewReader(data)); e != nil || o.putCount != 1 {
		t.Fatal("replay wrote again", e)
	}
	if _, _, e = m.Read(ctx, 8, u.AssetID); !errors.Is(e, ErrResourceHidden) {
		t.Fatal("private read allowed")
	}
}
func TestMediaSQLAndS3Failure(t *testing.T) {
	for _, kind := range []string{"s3", "sql-after-put", "ambiguous-commit"} {
		t.Run(kind, func(t *testing.T) {
			m, r, o, data := fixtureMedia(t)
			if kind == "s3" {
				o.putErr = errors.New("S3 down")
			} else if kind == "sql-after-put" {
				o.afterPut = func() { r.failCommit = true }
			} else {
				o.afterPut = func() { r.failCommit = true; r.ambiguous = true }
			}
			_, e := m.Upload(context.Background(), 7, 1, "image/jpeg", int64(len(data)), bytes.NewReader(data))
			if e == nil || (kind != "ambiguous-commit" && r.user.AvatarMediaID() != 0) {
				t.Fatal("failed replacement committed", e)
			}
			if kind != "s3" && len(o.stored) != 1 {
				t.Fatal("candidate deleted on ambiguous commit")
			}
			r.failCommit = false
			if kind == "ambiguous-commit" {
				if r.user.AvatarMediaID() != r.u.AssetID {
					t.Fatal("ambiguous commit was not simulated")
				}
				if _, e = m.Upload(context.Background(), 7, 1, "image/jpeg", int64(len(data)), bytes.NewReader(data)); e != nil || o.putCount != 1 {
					t.Fatal("ambiguous retry duplicated write", e)
				}
				if e = m.Cleanup(context.Background()); e != nil || len(o.stored) != 1 {
					t.Fatal("ambiguous successful commit deleted live asset", e)
				}
				return
			}
			if e = m.Cleanup(context.Background()); e != nil || len(o.stored) != 0 {
				t.Fatal("cleanup failed", e)
			}
		})
	}
}
func TestMediaTargetContract(t *testing.T) {
	base := MediaRequest{Purpose: "user_avatar", ContentType: "image/jpeg", SizeBytes: 1}
	for _, change := range []func(*MediaRequest){func(r *MediaRequest) { r.Purpose = "" }, func(r *MediaRequest) { r.BrandID = 1 }, func(r *MediaRequest) { r.ProductID = 1 }, func(r *MediaRequest) { r.ColorID = 1 }, func(r *MediaRequest) { v := int64(2); r.ImageID = &v }, func(r *MediaRequest) { r.Purpose = "brand_logo" }, func(r *MediaRequest) { r.Purpose = "product_image" }} {
		r := base
		change(&r)
		if _, e := r.target(7); e == nil {
			t.Fatal("ambiguous target allowed", r)
		}
	}
}

func TestMediaDeliveryReservesUploadAndBoundsGallery(t *testing.T) {
	m, _, _, data := fixtureMedia(t)
	if _, e := m.Upload(context.Background(), 7, 1, "image/jpeg", int64(len(data)), bytes.NewReader(data)); e != nil {
		t.Fatal(e)
	}
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	done := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			done <- m.Deliver(context.Background(), 7, 1, func(*MediaAsset, []byte) { entered <- struct{}{}; <-release })
		}()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			close(release)
			t.Fatal("delivery did not enter")
		}
	}
	_, e := m.Upload(context.Background(), 7, 1, "image/jpeg", int64(len(data)), bytes.NewReader(data))
	if e != nil {
		close(release)
		t.Fatal("slow readers blocked upload", e)
	}
	e = m.Deliver(context.Background(), 7, 1, func(*MediaAsset, []byte) { t.Error("unbounded gallery") })
	if !errors.Is(e, ErrMediaBusy) {
		close(release)
		t.Fatal("unbounded slow-reader buffers", e)
	}
	close(release)
	for i := 0; i < 2; i++ {
		if e := <-done; e != nil {
			t.Fatal(e)
		}
	}
}

func TestMediaBucketMismatchFailsBeforeObjectOperations(t *testing.T) {
	m, r, o, data := fixtureMedia(t)
	r.u.Asset.Bucket = "old-bucket"
	if _, e := m.Upload(context.Background(), 7, 1, "image/jpeg", int64(len(data)), bytes.NewReader(data)); !errors.Is(e, ErrMediaBucketMismatch) {
		t.Fatal(e)
	}
	o.stored[r.u.Asset.ObjectKey] = []byte("old bucket file")
	if e := m.Cleanup(context.Background()); !errors.Is(e, ErrMediaBucketMismatch) {
		t.Fatal(e)
	}
	if r.u.Asset.State == "deleted" || o.putCount != 0 || len(o.stored) == 0 {
		t.Fatal("mismatch changed old object")
	}
	r.u.Asset.Bucket = m.bucket
	if _, e := m.Upload(context.Background(), 7, 1, "image/jpeg", int64(len(data)), bytes.NewReader(data)); e != nil {
		t.Fatal(e)
	}
	r.u.Asset.Bucket = "old-bucket"
	if _, _, e := m.Read(context.Background(), 7, 1); !errors.Is(e, ErrMediaBucketMismatch) {
		t.Fatal("read wrong bucket", e)
	}
}

type cancelledMediaReader struct {
	ctx     context.Context
	entered chan struct{}
}

func (r cancelledMediaReader) Read([]byte) (int, error) {
	close(r.entered)
	<-r.ctx.Done()
	return 0, r.ctx.Err()
}
func TestMediaUploadSlotReleasedAfterCancellation(t *testing.T) {
	m, _, _, data := fixtureMedia(t)
	ctx, cancel := context.WithCancel(context.Background())
	entered := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, e := m.Upload(ctx, 7, 1, "image/jpeg", int64(len(data)), cancelledMediaReader{ctx, entered})
		done <- e
	}()
	<-entered
	if _, e := m.Upload(context.Background(), 7, 1, "image/jpeg", int64(len(data)), bytes.NewReader(data)); !errors.Is(e, ErrMediaBusy) {
		t.Fatal("unbounded decoder admissions", e)
	}
	cancel()
	if e := <-done; e == nil {
		t.Fatal("cancelled upload succeeded")
	}
	// Rejected request is not reusable, but it must reach the repository rather
	// than fail admission: the slot is released even after read/cancel errors.
	if _, e := m.Upload(context.Background(), 7, 1, "image/jpeg", int64(len(data)), bytes.NewReader(data)); !errors.Is(e, ErrConflict) {
		t.Fatal("cancel leaked decoder slot", e)
	}
}
