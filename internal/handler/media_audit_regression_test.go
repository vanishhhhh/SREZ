package handler

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"srez/internal/objectstorage"
	"srez/internal/storage"
	"srez/internal/usecase"
)

func auditAdapter(t *testing.T) (*objectstorage.S3, *s3.Client) {
	t.Helper()
	endpoint := os.Getenv("TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("isolated S3 required")
	}
	c := objectstorage.Config{Endpoint: endpoint, PublicEndpoint: endpoint, Region: "us-east-1", Bucket: "srez-media", AccessKey: os.Getenv("SREZ_S3_ACCESS_KEY"), SecretKey: os.Getenv("SREZ_S3_SECRET_KEY")}
	adapter, e := objectstorage.New(c)
	if e != nil {
		t.Fatal(e)
	}
	if e = adapter.EnsureBucket(context.Background()); e != nil {
		t.Fatal(e)
	}
	client := s3.NewFromConfig(aws.Config{Region: c.Region, Credentials: credentials.NewStaticCredentialsProvider(c.AccessKey, c.SecretKey, ""), HTTPClient: &http.Client{Timeout: 10 * time.Second}}, func(o *s3.Options) { o.BaseEndpoint = aws.String(c.Endpoint); o.UsePathStyle = true })
	return adapter, client
}
func auditUpload(t *testing.T, c pvmHTTPClient, r usecase.MediaRequest, data []byte) usecase.MediaUpload {
	t.Helper()
	metadata, _ := json.Marshal(r)
	var created struct {
		Upload usecase.MediaUpload `json:"upload"`
	}
	w := c.call("POST", "/media/uploads", string(metadata), 201)
	if e := json.Unmarshal(w.Body.Bytes(), &created); e != nil {
		t.Fatal(e)
	}
	req := httptest.NewRequest("PUT", fmt.Sprintf("/media/uploads/%d/content", created.Upload.ID), bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", r.ContentType)
	reply := httptest.NewRecorder()
	c.h.router.ServeHTTP(reply, req)
	if reply.Code != 200 {
		t.Fatalf("audit upload status %d: %s", reply.Code, reply.Body.String())
	}
	var u usecase.MediaUpload
	if e := json.Unmarshal(reply.Body.Bytes(), &u); e != nil {
		t.Fatal(e)
	}
	return u
}

type auditGetSignal struct {
	*objectstorage.S3
	entered chan struct{}
}

func (o *auditGetSignal) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	b, e := o.S3.Get(ctx, key)
	if e == nil {
		o.entered <- struct{}{}
	}
	return b, e
}
func TestMediaSlowPublicTCPDoesNotStarveUpload(t *testing.T) {
	adapter, _ := auditAdapter(t)
	if e := adapter.RetireStaging(context.Background()); e != nil {
		t.Fatal(e)
	}
	h, db := newPVMHTTP(t)
	objects := &auditGetSignal{S3: adapter, entered: make(chan struct{}, 8)}
	media := usecase.NewMedia(storage.NewMedia(db), objects, "srez-media", h.management)
	h.SetMedia(media)
	owner, _ := pvmAccount(t, h, "audit_starve_owner")
	brand := pvmDecode[brandResponse](t, owner.call("POST", "/brands", `{"name":"Audit brand","slug":"audit-starve"}`, 201))
	// A valid, incompressible alpha PNG below the source limit, large enough to
	// block TCP Write when an anonymous client stops reading its response body.
	img := image.NewNRGBA(image.Rect(0, 0, 1100, 1100))
	var rnd uint32 = 1234567
	for i := range img.Pix {
		rnd ^= rnd << 13
		rnd ^= rnd >> 17
		rnd ^= rnd << 5
		img.Pix[i] = byte(rnd)
	}
	var p bytes.Buffer
	if e := png.Encode(&p, img); e != nil {
		t.Fatal(e)
	}
	if int64(p.Len()) > usecase.MaxMediaBytes {
		t.Fatal("fixture too large")
	}
	u := auditUpload(t, owner, usecase.MediaRequest{Purpose: "brand_logo", BrandID: brand.ID, ContentType: "image/png", SizeBytes: int64(p.Len())}, p.Bytes())
	owner.call("PATCH", fmt.Sprintf("/brands/%d", brand.ID), `{"description":"Audit public brand"}`, 200)
	owner.call("POST", fmt.Sprintf("/brands/%d/publish", brand.ID), `{}`, 200)
	// Reserve a legitimate upload BEFORE the anonymous requests occupy slots.
	j, _, _ := mediaTestSources(t)
	meta, _ := json.Marshal(usecase.MediaRequest{Purpose: "user_avatar", ContentType: "image/jpeg", SizeBytes: int64(len(j))})
	var reserved struct {
		Upload usecase.MediaUpload `json:"upload"`
	}
	w := owner.call("POST", "/media/uploads", string(meta), 201)
	_ = json.Unmarshal(w.Body.Bytes(), &reserved)
	server := httptest.NewServer(h.router)
	defer server.Close()
	host := strings.TrimPrefix(server.URL, "http://")
	clients := []net.Conn{}
	defer func() {
		for _, c := range clients {
			c.Close()
		}
	}()
	for i := 0; i < 2; i++ {
		c, e := net.Dial("tcp", host)
		if e != nil {
			t.Fatal(e)
		}
		clients = append(clients, c)
		_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
		fmt.Fprintf(c, "GET /media/assets/%d/content HTTP/1.1\r\nHost: %s\r\n\r\n", u.AssetID, host)
		response, e := http.ReadResponse(bufio.NewReader(c), nil)
		if e != nil {
			t.Fatal(e)
		}
		if response.StatusCode != 200 {
			t.Fatal("guest read", response.StatusCode)
		}
	}
	for i := 0; i < 2; i++ {
		select {
		case <-objects.entered:
		case <-time.After(5 * time.Second):
			t.Fatal("GET did not enter")
		}
	}
	request := httptest.NewRequest("PUT", fmt.Sprintf("/media/uploads/%d/content", reserved.Upload.ID), bytes.NewReader(j))
	request.Header.Set("Authorization", "Bearer "+owner.token)
	request.Header.Set("Content-Type", "image/jpeg")
	result := httptest.NewRecorder()
	h.router.ServeHTTP(result, request)
	if result.Code != 200 {
		t.Fatalf("slow guests blocked upload: %d %s", result.Code, result.Body.String())
	}
	// A gallery burst is rejected immediately while the two bodies remain held.
	for i := 0; i < 20; i++ {
		r, e := http.Get(server.URL + fmt.Sprintf("/media/assets/%d/content", u.AssetID))
		if e != nil {
			t.Fatal(e)
		}
		r.Body.Close()
		if r.StatusCode != 429 {
			t.Fatalf("unbounded gallery: %d", r.StatusCode)
		}
	}
	t.Logf("two stalled public TCP readers: upload=%d; gallery burst=429; PNG=%d", result.Code, p.Len())
	for _, c := range clients {
		c.Close()
	}
	clients = nil
	deadline := time.Now().Add(3 * time.Second)
	for {
		response, e := http.Get(server.URL + fmt.Sprintf("/media/assets/%d/content", u.AssetID))
		if e != nil {
			t.Fatal(e)
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if response.StatusCode == 200 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("disconnect leaked delivery slots")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
func TestMediaConfiguredBucketChangeFailsSafely(t *testing.T) {
	first, _ := auditAdapter(t)
	ctx := context.Background()
	h, db := newPVMHTTP(t)
	initial := usecase.NewMedia(storage.NewMedia(db), first, "srez-media", h.management)
	h.SetMedia(initial)
	owner, uid := pvmAccount(t, h, "audit_bucket_owner")
	j, _, _ := mediaTestSources(t)
	r := usecase.MediaRequest{Purpose: "user_avatar", ContentType: "image/jpeg", SizeBytes: int64(len(j))}
	old := auditUpload(t, owner, r, j)
	var oldKey string
	if e := db.Get(&oldKey, `SELECT object_key FROM media_assets WHERE id=$1`, old.AssetID); e != nil {
		t.Fatal(e)
	}
	endpoint := os.Getenv("TEST_S3_ENDPOINT")
	second, e := objectstorage.New(objectstorage.Config{Endpoint: endpoint, PublicEndpoint: endpoint, Region: "us-east-1", Bucket: "audit-second-media", AccessKey: os.Getenv("SREZ_S3_ACCESS_KEY"), SecretKey: os.Getenv("SREZ_S3_SECRET_KEY")})
	if e != nil {
		t.Fatal(e)
	}
	if e = second.EnsureBucket(ctx); e != nil {
		t.Fatal(e)
	}
	reconfigured := usecase.NewMedia(storage.NewMedia(db), second, "audit-second-media", h.management)
	h.SetMedia(reconfigured)
	if _, _, e = reconfigured.Read(ctx, uid, old.AssetID); !errors.Is(e, usecase.ErrMediaBucketMismatch) {
		t.Fatal("wrong-bucket read", e)
	}
	_ = auditUpload(t, owner, r, j)
	if _, e = db.Exec(`UPDATE media_assets SET delete_after=now()-interval '1 hour' WHERE id=$1`, old.AssetID); e != nil {
		t.Fatal(e)
	}
	// Reads and existing pending uploads refuse the new configuration too.

	if e = second.Put(ctx, oldKey, "image/jpeg", bytes.NewReader(j)); e != nil {
		t.Fatal(e)
	}
	if e = reconfigured.Cleanup(ctx); !errors.Is(e, usecase.ErrMediaBucketMismatch) {
		t.Fatal("missing explicit configuration error", e)
	}
	if _, e = second.Head(ctx, oldKey); e != nil {
		t.Fatal("deleted matching key in new bucket", e)
	}
	var state, bucket string
	if e = db.QueryRow(`SELECT state,bucket FROM media_assets WHERE id=$1`, old.AssetID).Scan(&state, &bucket); e != nil {
		t.Fatal(e)
	}
	if state == "deleted" || bucket != "srez-media" {
		t.Fatal(state, bucket)
	}
	if _, e = first.Head(ctx, oldKey); e != nil {
		t.Fatal("recorded bucket object was actually deleted", e)
	}
	t.Log("A-to-B: explicit config error, old row not deleted, both bucket objects intact")
}

func TestMediaGCPrioritizesMandatoryAndBoundsReconciliation(t *testing.T) {
	adapter, _ := auditAdapter(t)
	h, db := newPVMHTTP(t)
	objects := &mediaS3Recorder{S3: adapter}
	repo := storage.NewMedia(db)
	m := usecase.NewMedia(repo, objects, "srez-media", h.management)
	h.SetMedia(m)
	owner, uid := pvmAccount(t, h, "gc_priority")
	j, _, _ := mediaTestSources(t)
	live := auditUpload(t, owner, usecase.MediaRequest{Purpose: "user_avatar", ContentType: "image/jpeg", SizeBytes: int64(len(j))}, j)
	_, e := db.Exec(`INSERT INTO media_assets(initiated_by,purpose,target_user_id,bucket,object_key,state,created_at,expires_at,deleted_at,reconcile_until,resweep_after) SELECT $1,'user_avatar',$1,'srez-media','assets/history-'||g,'deleted',now()-interval '2 days',now()-interval '2 days',now()-interval '1 hour',now()+interval '23 hours',now()-interval '1 hour' FROM generate_series(1,6100) g`, uid)
	if e != nil {
		t.Fatal(e)
	}
	var fresh int64
	if e = db.Get(&fresh, `INSERT INTO media_assets(initiated_by,purpose,target_user_id,bucket,object_key,state,expires_at) VALUES($1,'user_avatar',$1,'srez-media','assets/new-expired','processing',now()-interval '1 minute') RETURNING id`, uid); e != nil {
		t.Fatal(e)
	}
	rows, e := repo.Garbage(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, r := range rows {
		if r.ID == fresh {
			found = true
		}
		if r.ID == live.AssetID {
			t.Fatal("GC selected live asset")
		}
	}
	if !found || len(rows) != 11 {
		t.Fatal("history displaced mandatory work or exceeded separate budget", len(rows))
	}
	stats, e := repo.GCBacklog(context.Background())
	if e != nil || stats.Pending != 11 || stats.Reconciliation != 6090 || stats.OldestSeconds < 60 {
		t.Fatal("GC backlog measurement", stats, e)
	}
	// End the historical window; retained rows must never generate S3 work again.
	if _, e = db.Exec(`UPDATE media_assets SET reconcile_until=now()-interval '1 second' WHERE object_key LIKE 'assets/history-%'`); e != nil {
		t.Fatal(e)
	}
	objects.deleteFailure = true
	if e = m.Cleanup(context.Background()); e == nil {
		t.Fatal("DELETE failure ignored")
	}
	var state string
	if e = db.Get(&state, `SELECT state FROM media_assets WHERE id=$1`, fresh); e != nil || state != "deleting" {
		t.Fatal("failed DELETE marked complete", state, e)
	}
	objects.deleteFailure = false
	if e = m.Cleanup(context.Background()); e != nil {
		t.Fatal(e)
	}
	// Model a late provider PUT after the first successful deletion.
	if e = adapter.Put(context.Background(), "assets/new-expired", "image/jpeg", bytes.NewReader(j)); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`UPDATE media_assets SET resweep_after=now()-interval '1 second' WHERE id=$1`, fresh); e != nil {
		t.Fatal(e)
	}
	objects.deleteFailure = true
	if e = m.Cleanup(context.Background()); e == nil {
		t.Fatal("reconciliation DELETE failure ignored")
	}
	if e = db.Get(&state, `SELECT state FROM media_assets WHERE id=$1`, fresh); e != nil || state != "deleting" {
		t.Fatal("failed resweep not durably retried", state, e)
	}
	// Once a late-object cleanup is claimed, failure remains mandatory even if
	// its finite history reconciliation window ends during the outage.
	if _, e = db.Exec(`UPDATE media_assets SET reconcile_until=now()-interval '1 second' WHERE id=$1`, fresh); e != nil {
		t.Fatal(e)
	}
	objects.deleteFailure = false
	if e = m.Cleanup(context.Background()); e != nil {
		t.Fatal(e)
	}
	if _, e = adapter.Head(context.Background(), "assets/new-expired"); e == nil {
		t.Fatal("late object survived reconciliation")
	}
	var liveKey string
	if e = db.Get(&liveKey, `SELECT object_key FROM media_assets WHERE id=$1`, live.AssetID); e != nil {
		t.Fatal(e)
	}
	if _, e = adapter.Head(context.Background(), liveKey); e != nil {
		t.Fatal("live object lost", e)
	}
	if _, e = db.Exec(`UPDATE media_assets SET reconcile_until=now()-interval '1 second',resweep_after=now()-interval '1 second' WHERE id=$1`, fresh); e != nil {
		t.Fatal(e)
	}
	rows, e = repo.Garbage(context.Background())
	if e != nil || len(rows) != 0 {
		t.Fatal("unbounded historical resweep", len(rows), e)
	}
	var count int
	if e = db.Get(&count, `SELECT count(*) FROM media_assets WHERE state='deleted'`); e != nil || count != 6101 {
		t.Fatal("audit history removed", count, e)
	}
	t.Log("6100 tombstones: mandatory selected, resweep<=10, live ref safe, DELETE retried, late PUT removed, expired window=0 work")
}

func TestMediaUploadAdmissionReleasesOnTCPDisconnectAndTimeout(t *testing.T) {
	adapter, _ := auditAdapter(t)
	h, db := newPVMHTTP(t)
	h.SetMedia(usecase.NewMedia(storage.NewMedia(db), adapter, "srez-media", h.management))
	owner, _ := pvmAccount(t, h, "upload_slot_release")
	j, _, _ := mediaTestSources(t)
	reserve := func() int64 {
		t.Helper()
		meta, _ := json.Marshal(usecase.MediaRequest{Purpose: "user_avatar", ContentType: "image/jpeg", SizeBytes: int64(len(j))})
		var r struct {
			Upload usecase.MediaUpload `json:"upload"`
		}
		w := owner.call("POST", "/media/uploads", string(meta), 201)
		if e := json.Unmarshal(w.Body.Bytes(), &r); e != nil {
			t.Fatal(e)
		}
		return r.Upload.ID
	}
	send := func(id int64) int {
		r := httptest.NewRequest("PUT", fmt.Sprintf("/media/uploads/%d/content", id), bytes.NewReader(j))
		r.Header.Set("Authorization", "Bearer "+owner.token)
		r.Header.Set("Content-Type", "image/jpeg")
		w := httptest.NewRecorder()
		h.router.ServeHTTP(w, r)
		return w.Code
	}
	server := httptest.NewServer(h.router)
	defer server.Close()
	host := strings.TrimPrefix(server.URL, "http://")
	for _, mode := range []string{"disconnect", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			stalled, next := reserve(), reserve()
			conn, e := net.Dial("tcp", host)
			if e != nil {
				t.Fatal(e)
			}
			defer conn.Close()
			conn.SetDeadline(time.Now().Add(15 * time.Second))
			fmt.Fprintf(conn, "PUT /media/uploads/%d/content HTTP/1.1\r\nHost: %s\r\nAuthorization: Bearer %s\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n", stalled, host, owner.token, len(j))
			conn.Write(j[:10])
			deadline := time.Now().Add(3 * time.Second)
			for {
				var state string
				if e = db.Get(&state, `SELECT state FROM media_upload_requests WHERE id=$1`, stalled); e != nil {
					t.Fatal(e)
				}
				if state == "processing" {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("stalled upload never claimed slot")
				}
				time.Sleep(10 * time.Millisecond)
			}
			if code := send(next); code != 429 {
				t.Fatal("heavy upload budget exceeded", code)
			}
			if mode == "disconnect" {
				conn.Close()
			} else {
				r, e := http.ReadResponse(bufio.NewReader(conn), nil)
				if e != nil {
					t.Fatal("read deadline did not return response", e)
				}
				r.Body.Close()
				if r.StatusCode != 400 {
					t.Fatal("stalled source not rejected", r.StatusCode)
				}
			}
			deadline = time.Now().Add(3 * time.Second)
			for {
				code := send(next)
				if code == 200 {
					break
				}
				if code != 429 || time.Now().After(deadline) {
					t.Fatal("upload slot leaked", code)
				}
				time.Sleep(10 * time.Millisecond)
			}
		})
	}
}

func TestMediaDeliverySignalsTemporaryStorageFailure(t *testing.T) {
	adapter, _ := auditAdapter(t)
	h, db := newPVMHTTP(t)
	media := usecase.NewMedia(storage.NewMedia(db), adapter, "srez-media", h.management)
	h.SetMedia(media)
	owner, _ := pvmAccount(t, h, "audit_retry_owner")
	data, _, _ := mediaTestSources(t)
	asset := auditUpload(t, owner, usecase.MediaRequest{
		Purpose: "user_avatar", ContentType: "image/jpeg", SizeBytes: int64(len(data)),
	}, data)
	var key string
	if err := db.Get(&key, `SELECT object_key FROM media_assets WHERE id=$1`, asset.AssetID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = adapter.Delete(context.Background(), key) })
	path := fmt.Sprintf("/media/assets/%d/content", asset.AssetID)
	baseline := owner.call("GET", path, "", http.StatusOK)
	outage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "private storage diagnostics", http.StatusServiceUnavailable)
	}))
	defer outage.Close()
	failed, err := objectstorage.New(objectstorage.Config{
		Endpoint: outage.URL, PublicEndpoint: outage.URL, Region: "us-east-1",
		Bucket: "srez-media", AccessKey: "temporary-test-key", SecretKey: "temporary-test-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	h.SetMedia(usecase.NewMedia(storage.NewMedia(db), failed, "srez-media", h.management))
	response := owner.call("GET", path, "", http.StatusServiceUnavailable)
	if response.Header().Get("Retry-After") != "1" || response.Header().Get("Cache-Control") != "no-store" ||
		strings.Contains(response.Body.String(), "private storage diagnostics") {
		t.Fatal("unsafe temporary delivery response")
	}
	guest := pvmHTTPClient{t: t, h: h}
	guest.call("GET", path, "", http.StatusNotFound)
	h.SetMedia(media)
	if recovered := owner.call("GET", path, "", http.StatusOK); !bytes.Equal(recovered.Body.Bytes(), baseline.Body.Bytes()) {
		t.Fatal("delivery changed the original media")
	}
}
