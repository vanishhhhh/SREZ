package storage_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"srez/internal/objectstorage"
	"srez/internal/storage"
	"srez/internal/usecase"
	"srez/migrations"
)

func verifyMediaPayload(body io.Reader, size int64, want [32]byte) error {
	if size <= 0 || size > usecase.MaxMediaOutputBytes {
		return errors.New("invalid size")
	}
	data, e := io.ReadAll(io.LimitReader(body, usecase.MaxMediaOutputBytes+1))
	if e != nil {
		return e
	}
	if int64(len(data)) != size || sha256.Sum256(data) != want {
		return errors.New("payload length or SHA-256 mismatch")
	}
	return nil
}
func TestPersistenceRejectsTruncatedAndCorruptedHTTPBody(t *testing.T) {
	want := sha256.Sum256([]byte("expected payload"))
	for _, body := range []string{"x", "changed payload!"} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Length", "16")
				io.WriteString(w, body)
			}))
			defer server.Close()
			response, e := http.Get(server.URL)
			if e != nil {
				t.Fatal(e)
			}
			defer response.Body.Close()
			if e = verifyMediaPayload(response.Body, 16, want); e == nil {
				t.Fatal("false positive persistence")
			}
		})
	}
	if e := verifyMediaPayload(bytes.NewReader([]byte("expected payload")), 16, want); e != nil {
		t.Fatal(e)
	}
}

// Cross-process fixture in a unique schema of the isolated Compose PostgreSQL.
// It is a normalized ready avatar with its actual SQL domain reference, not a
// detached copy. Expected size/hash are recorded before the volume restart.
func TestSeaweedPersistenceAfterRestart(t *testing.T) {
	write := os.Getenv("TEST_S3_WRITE_RESTART") == "true"
	if !write && os.Getenv("TEST_S3_VERIFY_RESTART") != "true" {
		t.Skip("restart verification not requested")
	}
	schema := os.Getenv("TEST_MEDIA_PERSISTENCE_SCHEMA")
	if !strings.HasPrefix(schema, "media_restart_") {
		t.Fatal("unique isolated restart schema required")
	}
	endpoint := os.Getenv("TEST_S3_ENDPOINT")
	s, e := objectstorage.New(objectstorage.Config{Endpoint: endpoint, PublicEndpoint: endpoint, Region: "us-east-1", Bucket: "srez-media", AccessKey: os.Getenv("SREZ_S3_ACCESS_KEY"), SecretKey: os.Getenv("SREZ_S3_SECRET_KEY")})
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	dsn := os.Getenv("TEST_DATABASE_URL")
	admin, e := sqlx.Connect("postgres", dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer admin.Close()
	if write {
		if _, e = admin.Exec(`CREATE SCHEMA ` + pq.QuoteIdentifier(schema)); e != nil {
			t.Fatal(e)
		}
	}
	u, e := url.Parse(dsn)
	if e != nil {
		t.Fatal(e)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, e := sqlx.Connect("postgres", u.String())
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	m := usecase.NewMedia(storage.NewMedia(db), s, "srez-media")
	var id, uid, size int64
	var key, hash string
	if write {
		if e = migrations.Apply(ctx, db); e != nil {
			t.Fatal(e)
		}
		if e = db.Get(&uid, `INSERT INTO users(email,username,display_name,password_hash,created_at,updated_at) VALUES('restart@example.com','restart','Restart','!',now(),now()) RETURNING id`); e != nil {
			t.Fatal(e)
		}
		img := image.NewNRGBA(image.Rect(0, 0, 1100, 1100))
		var rnd uint32 = 1234567
		for i := range img.Pix {
			rnd ^= rnd << 13
			rnd ^= rnd >> 17
			rnd ^= rnd << 5
			img.Pix[i] = byte(rnd)
		}
		var src bytes.Buffer
		if e = png.Encode(&src, img); e != nil {
			t.Fatal(e)
		}
		pending, e := m.Create(ctx, uid, usecase.MediaRequest{Purpose: "user_avatar", ContentType: "image/png", SizeBytes: int64(src.Len())})
		if e != nil {
			t.Fatal(e)
		}
		ready, e := m.Upload(ctx, uid, pending.ID, "image/png", int64(src.Len()), bytes.NewReader(src.Bytes()))
		if e != nil {
			t.Fatal(e)
		}
		a, data, e := m.Read(ctx, uid, ready.AssetID)
		if e != nil {
			t.Fatal(e)
		}
		id = a.ID
		size = int64(len(data))
		key = a.ObjectKey
		hash = fmt.Sprintf("%x", sha256.Sum256(data))
		exec(t, db, `CREATE TABLE restart_expected(asset_id bigint,user_id bigint,object_key text,size_bytes bigint,sha256 text)`)
		exec(t, db, `INSERT INTO restart_expected VALUES($1,$2,$3,$4,$5)`, id, uid, key, size, hash)
	} else {
		if e = db.QueryRow(`SELECT asset_id,user_id,object_key,size_bytes,sha256 FROM restart_expected`).Scan(&id, &uid, &key, &size, &hash); e != nil {
			t.Fatal(e)
		}
	}
	var digest [32]byte
	decoded, e := hex.DecodeString(hash)
	if e != nil || len(decoded) != 32 {
		t.Fatal("invalid expected digest", e)
	}
	copy(digest[:], decoded)
	// SeaweedFS may take a brief period to reload volume metadata after healthy.
	var info usecase.ObjectInfo
	for attempt := 0; attempt < 30; attempt++ {
		info, e = s.Head(ctx, key)
		if e == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(500 * time.Millisecond):
		}
	}
	if e != nil || info.Size != size {
		t.Fatal("HEAD mismatch", e, info.Size, size)
	}
	body, e := s.Get(ctx, key)
	if e != nil {
		t.Fatal(e)
	}
	defer body.Close()
	if e = verifyMediaPayload(body, size, digest); e != nil {
		t.Fatal("full S3 GET verification", e)
	}
	a, data, e := m.Read(ctx, uid, id)
	if e != nil {
		t.Fatal("referenced ready asset unavailable", e)
	}
	if a.State != "ready" || int64(len(data)) != size || fmt.Sprintf("%x", sha256.Sum256(data)) != hash {
		t.Fatal("ready SQL reference/payload mismatch")
	}
	t.Logf("ready asset=%d bytes=%d SHA-256=%s write_before_restart=%v", id, size, hash, write)
}
