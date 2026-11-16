package usecase

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"slices"
	"time"

	_ "golang.org/x/image/webp"
	"srez/internal/entities"
)

const MaxMediaBytes int64 = 5 * 1024 * 1024
const MaxMediaOutputBytes int64 = 8 * 1024 * 1024
const MaxMediaPixels = 12_000_000

var ErrMediaTooLarge = errors.New("media exceeds size limit")
var ErrMediaBucketMismatch = errors.New("media storage bucket configuration mismatch")
var ErrMediaBusy = errors.New("media processing busy")

// UploadURL remains an adapter capability, not an externally reachable upload flow.
type UploadURL struct {
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers"`
	ExpiresAt time.Time         `json:"expires_at"`
}
type ObjectInfo struct {
	Size int64
	MIME string
}
type MediaObjects interface {
	Ready(context.Context) error
	Get(context.Context, string) (io.ReadCloser, error)
	Put(context.Context, string, string, io.Reader) error
	Delete(context.Context, string) error
}
type MediaTarget struct {
	Purpose      string `db:"purpose" json:"purpose"`
	TargetUserID int64  `db:"target_user_id" json:"-"`
	BrandID      int64  `db:"brand_id" json:"brand_id,omitempty"`
	ProductID    int64  `db:"product_id" json:"product_id,omitempty"`
	ColorID      int64  `db:"color_id" json:"color_id,omitempty"`
}
type MediaAsset struct {
	ID          int64 `db:"id" json:"id"`
	InitiatedBy int64 `db:"initiated_by" json:"-"`
	MediaTarget
	Bucket      string     `db:"bucket" json:"-"`
	ObjectKey   string     `db:"object_key" json:"-"`
	State       string     `db:"state" json:"state"`
	MIME        string     `db:"mime" json:"content_type,omitempty"`
	Size        int64      `db:"size_bytes" json:"size_bytes,omitempty"`
	CreatedAt   time.Time  `db:"created_at" json:"created_at"`
	ConfirmedAt *time.Time `db:"confirmed_at" json:"confirmed_at,omitempty"`
	ExpiresAt   time.Time  `db:"expires_at" json:"expires_at"`
	DeleteAfter *time.Time `db:"delete_after" json:"-"`
}
type MediaUpload struct {
	ID           int64      `db:"id" json:"id"`
	AssetID      int64      `db:"asset_id" json:"media_id"`
	InitiatedBy  int64      `db:"initiated_by" json:"-"`
	ExpectedSize int64      `db:"expected_size" json:"-"`
	ExpectedMIME string     `db:"expected_mime" json:"-"`
	SourceSize   *int64     `db:"source_size" json:"-"`
	ImageID      *int64     `db:"image_id" json:"image_id,omitempty"`
	Position     *int       `db:"position" json:"position,omitempty"`
	State        string     `db:"state" json:"state"`
	CreatedAt    time.Time  `db:"created_at" json:"created_at"`
	CompletedAt  *time.Time `db:"completed_at" json:"completed_at,omitempty"`
	Asset        MediaAsset `json:"asset"`
}
type MediaRequest struct {
	Purpose     string `json:"purpose"`
	BrandID     int64  `json:"brand_id,omitempty"`
	ProductID   int64  `json:"product_id,omitempty"`
	ColorID     int64  `json:"color_id,omitempty"`
	ImageID     *int64 `json:"image_id,omitempty"`
	Position    *int   `json:"position,omitempty"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
}

// MediaTx participates in the existing parent transaction and lock order.
// Implemented by managementTx, not a parallel RBAC transaction mechanism.
type MediaTx interface {
	CreateMediaUpload(context.Context, *MediaUpload) error
	LockMediaUpload(context.Context, int64, int64) (*MediaUpload, error)
	ClaimMediaUpload(context.Context, int64) error
	ReadyMediaUpload(context.Context, *MediaUpload) error
	SaveMediaUser(context.Context, entities.User) error
	SaveMediaBrand(context.Context, entities.Brand) error
}
type MediaRepository interface {
	WithinMediaUser(context.Context, int64, func(MediaTx, *entities.User) error) error
	Upload(context.Context, int64, int64) (*MediaUpload, error)
	RejectUpload(context.Context, int64) error
	ReadableAsset(context.Context, int64, int64) (*MediaAsset, error)
	Garbage(context.Context) ([]MediaAsset, error)
	Deleted(context.Context, int64) error
	CleanupLegacy(context.Context, MediaObjects, string) error
}
type Media struct {
	repo       MediaRepository
	objects    MediaObjects
	bucket     string
	management *Management
	uploads    chan struct{}
	deliveries chan struct{}
}

func NewMedia(r MediaRepository, o MediaObjects, bucket string, management ...*Management) *Media {
	m := &Media{repo: r, objects: o, bucket: bucket, uploads: make(chan struct{}, 1), deliveries: make(chan struct{}, 2)}
	if len(management) > 0 {
		m.management = management[0]
	}
	return m
}
func allowedMedia(m string) bool { return m == "image/jpeg" || m == "image/png" || m == "image/webp" }
func mediaKey() (string, error) {
	b := make([]byte, 32)
	_, e := rand.Read(b)
	return "assets/" + hex.EncodeToString(b), e
}
func (r MediaRequest) target(uid int64) (MediaTarget, error) {
	t := MediaTarget{Purpose: r.Purpose, BrandID: r.BrandID, ProductID: r.ProductID, ColorID: r.ColorID}
	if uid <= 0 || r.BrandID < 0 || r.ProductID < 0 || r.ColorID < 0 || !allowedMedia(r.ContentType) || r.SizeBytes <= 0 {
		return t, entities.ErrInvalidValue
	}
	if r.SizeBytes > MaxMediaBytes {
		return t, ErrMediaTooLarge
	}
	switch r.Purpose {
	case "user_avatar":
		if r.BrandID != 0 || r.ProductID != 0 || r.ColorID != 0 {
			return t, entities.ErrInvalidValue
		}
		t.TargetUserID = uid
	case "brand_logo", "brand_banner":
		if r.BrandID <= 0 || r.ProductID != 0 || r.ColorID != 0 {
			return t, entities.ErrInvalidValue
		}
	case "product_image":
		if r.BrandID != 0 || r.ProductID <= 0 || r.ColorID <= 0 {
			return t, entities.ErrInvalidValue
		}
	default:
		return t, entities.ErrInvalidValue
	}
	if r.ImageID != nil && r.Position != nil {
		return t, entities.ErrInvalidValue
	}
	if r.Purpose != "product_image" && (r.ImageID != nil || r.Position != nil) {
		return t, entities.ErrInvalidValue
	}
	if (r.ImageID != nil && *r.ImageID <= 0) || (r.Position != nil && (*r.Position < 0 || *r.Position >= 10)) {
		return t, entities.ErrInvalidValue
	}
	return t, nil
}

type mediaTargetContext struct {
	tx           MediaTx
	managementTx ManagementTx
	user         *entities.User
	brand        *entities.Brand
	product      *entities.Product
	data         entities.ProductPublication
}

func (m *Media) withinTarget(ctx context.Context, uid int64, t *MediaTarget, write bool, fn func(mediaTargetContext) error) error {
	if uid <= 0 {
		return ErrResourceHidden
	}
	if t.Purpose == "user_avatar" {
		if t.TargetUserID != uid {
			return ErrResourceHidden
		}
		return m.repo.WithinMediaUser(ctx, uid, func(tx MediaTx, u *entities.User) error { return fn(mediaTargetContext{tx: tx, user: u}) })
	}
	if m.management == nil {
		return ErrConflict
	}
	if t.Purpose == "product_image" {
		return m.management.withProduct(ctx, uid, t.ProductID, write, func(tx ManagementTx, p *entities.Product, data entities.ProductPublication) error {
			if _, e := managedColor(data, t.ColorID); e != nil {
				return e
			}
			if t.BrandID != 0 && t.BrandID != p.BrandID() {
				return ErrResourceHidden
			}
			t.BrandID = p.BrandID()
			mt, ok := tx.(MediaTx)
			if !ok {
				return ErrConflict
			}
			return fn(mediaTargetContext{tx: mt, managementTx: tx, product: p, data: data})
		})
	}
	return m.management.brand(ctx, uid, t.BrandID, true, func(tx ManagementTx, b *entities.Brand) error {
		if b.Status() == entities.BrandSuspended {
			return ErrResourceHidden
		}
		mt, ok := tx.(MediaTx)
		if !ok {
			return ErrConflict
		}
		return fn(mediaTargetContext{tx: mt, managementTx: tx, brand: b})
	})
}
func (m *Media) Create(ctx context.Context, uid int64, r MediaRequest) (*MediaUpload, error) {
	t, e := r.target(uid)
	if e != nil {
		return nil, e
	}
	key, e := mediaKey()
	if e != nil {
		return nil, e
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	u := &MediaUpload{InitiatedBy: uid, ExpectedSize: r.SizeBytes, ExpectedMIME: r.ContentType, ImageID: r.ImageID, Position: r.Position, Asset: MediaAsset{InitiatedBy: uid, MediaTarget: t, Bucket: m.bucket, ObjectKey: key}}
	if u.Asset.Bucket != m.bucket {
		return nil, ErrMediaBucketMismatch
	}
	e = m.withinTarget(ctx, uid, &u.Asset.MediaTarget, false, func(v mediaTargetContext) error {
		if r.Purpose == "product_image" {
			images, e := v.managementTx.Images(ctx, t.ColorID)
			if e != nil {
				return e
			}
			if r.ImageID != nil {
				if _, e = imageIndex(images, *r.ImageID); e != nil {
					return e
				}
			} else if len(images) >= 10 {
				return ErrConflict
			}
			if r.Position != nil && *r.Position > len(images) {
				return entities.ErrInvalidValue
			}
		}
		return v.tx.CreateMediaUpload(ctx, u)
	})
	return u, e
}

// Upload reads at most source limit+1 bytes, validates and normalizes BEFORE
// the first S3 PUT. A durable claim ensures a key is written only once.
func (m *Media) Upload(ctx context.Context, uid, id int64, mime string, length int64, body io.Reader) (*MediaUpload, error) {
	if length > MaxMediaBytes {
		return nil, ErrMediaTooLarge
	}
	if length < 0 {
		return nil, entities.ErrInvalidValue
	}
	select {
	case m.uploads <- struct{}{}:
		defer func() { <-m.uploads }()
	default:
		return nil, ErrMediaBusy
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	u, e := m.repo.Upload(ctx, uid, id)
	if e != nil {
		return nil, e
	}
	if u.Asset.Bucket != m.bucket {
		return nil, ErrMediaBucketMismatch
	}
	e = m.withinTarget(ctx, uid, &u.Asset.MediaTarget, false, func(v mediaTargetContext) error {
		var e error
		u, e = v.tx.LockMediaUpload(ctx, uid, id)
		if e != nil {
			return e
		}
		if u.Asset.Bucket != m.bucket {
			return ErrMediaBucketMismatch
		}
		if u.State == "complete" {
			return nil
		}
		if u.State != "pending" || !time.Now().Before(u.Asset.ExpiresAt) {
			return ErrConflict
		}
		if mime != u.ExpectedMIME || length != u.ExpectedSize {
			return entities.ErrInvalidValue
		}
		return v.tx.ClaimMediaUpload(ctx, id)
	})
	if e != nil {
		return nil, e
	}
	if u.State == "complete" {
		return u, nil
	}
	reject := func(err error) (*MediaUpload, error) {
		c, stop := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer stop()
		_ = m.repo.RejectUpload(c, id)
		return nil, err
	}
	data, e := io.ReadAll(io.LimitReader(body, MaxMediaBytes+1))
	if e != nil {
		var size *http.MaxBytesError
		if errors.As(e, &size) {
			return reject(ErrMediaTooLarge)
		}
		return reject(entities.ErrInvalidValue)
	}
	if int64(len(data)) > MaxMediaBytes {
		return reject(ErrMediaTooLarge)
	}
	if int64(len(data)) != length {
		return reject(entities.ErrInvalidValue)
	}
	normalized, outputMIME, e := NormalizeImage(data, mime)
	if e != nil {
		return reject(e)
	}
	if e = ctx.Err(); e != nil {
		return reject(e)
	}
	if e = m.objects.Ready(ctx); e != nil {
		return nil, e
	}
	if e = m.objects.Put(ctx, u.Asset.ObjectKey, outputMIME, bytes.NewReader(normalized)); e != nil {
		return nil, e
	}
	// No immediate DELETE on failure/ambiguous commit. The journal and GC decide.
	u.Asset.MIME = outputMIME
	u.Asset.Size = int64(len(normalized))
	sourceSize := int64(len(data))
	u.SourceSize = &sourceSize
	e = m.withinTarget(ctx, uid, &u.Asset.MediaTarget, true, func(v mediaTargetContext) error {
		current, e := v.tx.LockMediaUpload(ctx, uid, id)
		if e != nil {
			return e
		}
		if current.Asset.Bucket != m.bucket {
			return ErrMediaBucketMismatch
		}
		if current.State != "processing" || current.Asset.State != "processing" || !time.Now().Before(current.Asset.ExpiresAt) {
			return ErrConflict
		}
		if e = m.bind(ctx, v, u); e != nil {
			return e
		}
		return v.tx.ReadyMediaUpload(ctx, u)
	})
	if e != nil {
		return nil, e
	}
	return m.repo.Upload(ctx, uid, id)
}
func (m *Media) bind(ctx context.Context, v mediaTargetContext, u *MediaUpload) error {
	now := time.Now().UTC()
	id := u.AssetID
	switch u.Asset.Purpose {
	case "user_avatar":
		if now.Before(v.user.UpdatedAt()) {
			now = v.user.UpdatedAt()
		}
		if e := v.user.ChangeAvatarMedia(id, now); e != nil {
			return e
		}
		return v.tx.SaveMediaUser(ctx, *v.user)
	case "brand_logo", "brand_banner":
		if now.Before(v.brand.UpdatedAt()) {
			now = v.brand.UpdatedAt()
		}
		var e error
		if u.Asset.Purpose == "brand_logo" {
			e = v.brand.ChangeLogoMedia(id, now)
		} else {
			e = v.brand.ChangeBannerMedia(id, now)
		}
		if e != nil {
			return e
		}
		return v.tx.SaveMediaBrand(ctx, *v.brand)
	case "product_image":
		images, e := v.managementTx.Images(ctx, u.Asset.ColorID)
		if e != nil {
			return e
		}
		if u.ImageID != nil {
			index, e := imageIndex(images, *u.ImageID)
			if e != nil {
				return e
			}
			i := images[index]
			if e = i.ChangeMedia(id); e != nil {
				return e
			}
			return v.managementTx.SaveImage(ctx, i)
		}
		if len(images) >= 10 {
			return ErrConflict
		}
		position := len(images)
		if u.Position != nil {
			position = *u.Position
		}
		if position < 0 || position > len(images) {
			return entities.ErrInvalidValue
		}
		i, e := entities.NewManagedProductImage(u.Asset.ColorID, id, position)
		if e != nil {
			return e
		}
		i, e = v.managementTx.InsertImage(ctx, *i)
		if e != nil {
			return e
		}
		u.ImageID = new(int64)
		*u.ImageID = i.ID()
		images = slices.Insert(images, position, *i)
		return v.managementTx.SaveImagePositions(ctx, u.Asset.ColorID, imageIDs(images))
	}
	return entities.ErrInvalidValue
}
func (m *Media) DeleteTarget(ctx context.Context, uid int64, t MediaTarget) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if t.Purpose == "user_avatar" {
		t.TargetUserID = uid
	}
	if t.Purpose != "user_avatar" && t.Purpose != "brand_logo" && t.Purpose != "brand_banner" {
		return entities.ErrInvalidValue
	}
	return m.withinTarget(ctx, uid, &t, true, func(v mediaTargetContext) error {
		now := time.Now().UTC()
		if v.user != nil {
			if now.Before(v.user.UpdatedAt()) {
				now = v.user.UpdatedAt()
			}
			if e := v.user.ChangeAvatarMedia(0, now); e != nil {
				return e
			}
			return v.tx.SaveMediaUser(ctx, *v.user)
		}
		if now.Before(v.brand.UpdatedAt()) {
			now = v.brand.UpdatedAt()
		}
		var e error
		if t.Purpose == "brand_logo" {
			e = v.brand.ChangeLogoMedia(0, now)
		} else {
			e = v.brand.ChangeBannerMedia(0, now)
		}
		if e != nil {
			return e
		}
		return v.tx.SaveMediaBrand(ctx, *v.brand)
	})
}
func (m *Media) read(ctx context.Context, uid, id int64) (*MediaAsset, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	a, e := m.repo.ReadableAsset(ctx, uid, id)
	if e != nil {
		return nil, nil, e
	}
	if a.Bucket != m.bucket {
		return nil, nil, ErrMediaBucketMismatch
	}
	b, e := m.objects.Get(ctx, a.ObjectKey)
	if e != nil {
		return nil, nil, e
	}
	defer b.Close()
	data, e := io.ReadAll(io.LimitReader(b, MaxMediaOutputBytes+1))
	if e != nil {
		return nil, nil, e
	}
	if int64(len(data)) != a.Size || int64(len(data)) > MaxMediaOutputBytes {
		return nil, nil, ErrConflict
	}
	return a, data, nil
}

// One decoder plus two deliveries bound the entire process without a queue.
// Compared with two decoder slots this reserves upload capacity with less heavy
// work; delivery holds at most two bounded 8 MiB bodies (plus ReadAll capacity).
// Deliver retains its delivery slot until the HTTP response has
// finished writing. Slow readers cannot accumulate unbounded 8 MiB buffers.
func (m *Media) Deliver(ctx context.Context, uid, id int64, send func(*MediaAsset, []byte)) error {
	select {
	case m.deliveries <- struct{}{}:
		defer func() { <-m.deliveries }()
	default:
		return ErrMediaBusy
	}
	a, data, e := m.read(ctx, uid, id)
	if e != nil {
		return e
	}
	send(a, data)
	return nil
}
func (m *Media) Read(ctx context.Context, uid, id int64) (a *MediaAsset, data []byte, e error) {
	e = m.Deliver(ctx, uid, id, func(asset *MediaAsset, body []byte) { a = asset; data = body })
	return
}

// Output is JPEG for opaque images (quality 85), PNG for transparency. Both
// encoders are pure Go; a bounded writer caps the normalized object at 8 MiB.
type boundedImageBuffer struct{ buffer bytes.Buffer }

func (b *boundedImageBuffer) Bytes() []byte { return b.buffer.Bytes() }

func (b *boundedImageBuffer) Write(p []byte) (int, error) {
	if int64(b.buffer.Len()+len(p)) > MaxMediaOutputBytes {
		return 0, ErrMediaTooLarge
	}
	return b.buffer.Write(p)
}
func NormalizeImage(data []byte, mime string) ([]byte, string, error) {
	if int64(len(data)) > MaxMediaBytes {
		return nil, "", ErrMediaTooLarge
	}
	if len(data) == 0 || !allowedMedia(mime) || http.DetectContentType(data) != mime {
		return nil, "", entities.ErrInvalidValue
	}
	cfg, format, e := image.DecodeConfig(bytes.NewReader(data))
	if e != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 8192 || cfg.Height > 8192 || int64(cfg.Width)*int64(cfg.Height) > MaxMediaPixels {
		return nil, "", entities.ErrInvalidValue
	}
	if map[string]string{"image/jpeg": "jpeg", "image/png": "png", "image/webp": "webp"}[mime] != format {
		return nil, "", entities.ErrInvalidValue
	}
	img, _, e := image.Decode(bytes.NewReader(data))
	if e != nil {
		return nil, "", entities.ErrInvalidValue
	}
	opaque := true
	if v, ok := img.(interface{ Opaque() bool }); ok {
		opaque = v.Opaque()
	} else {
		bounds := img.Bounds()
		for y := bounds.Min.Y; y < bounds.Max.Y && opaque; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				_, _, _, a := img.At(x, y).RGBA()
				if a != 0xffff {
					opaque = false
					break
				}
			}
		}
	}
	var out boundedImageBuffer
	outputMIME := "image/png"
	if opaque {
		outputMIME = "image/jpeg"
		e = jpeg.Encode(&out, img, &jpeg.Options{Quality: 85})
	} else {
		e = png.Encode(&out, img)
	}
	if e != nil {
		return nil, "", e
	}
	return out.Bytes(), outputMIME, nil
}
func (m *Media) Cleanup(ctx context.Context) error {
	rows, e := m.repo.Garbage(ctx)
	if e != nil {
		return e
	}
	var failures []error
	for _, a := range rows {
		if a.Bucket != m.bucket {
			failures = append(failures, ErrMediaBucketMismatch)
			continue
		}
		if e = m.objects.Delete(ctx, a.ObjectKey); e != nil {
			failures = append(failures, e)
			continue
		}
		if e = m.repo.Deleted(ctx, a.ID); e != nil {
			failures = append(failures, e)
		}
	}
	if e = m.repo.CleanupLegacy(ctx, m.objects, m.bucket); e != nil {
		failures = append(failures, e)
	}
	return errors.Join(failures...)
}

type MediaGCBacklog struct {
	Pending        int64   `db:"pending"`
	OldestSeconds  float64 `db:"oldest_seconds"`
	Reconciliation int64   `db:"reconciliation"`
}

func (m *Media) RunCleanup(ctx context.Context, report func(error), observe ...func(MediaGCBacklog)) {
	timer := time.NewTicker(time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			c, cancel := context.WithTimeout(ctx, 45*time.Second)
			e := m.Cleanup(c)
			if stats, ok := m.repo.(interface {
				GCBacklog(context.Context) (MediaGCBacklog, error)
			}); ok && len(observe) > 0 {
				b, err := stats.GCBacklog(c)
				if err != nil {
					report(err)
				} else {
					observe[0](b)
				}
			}
			cancel()
			if e != nil {
				report(e)
			}
		}
	}
}
