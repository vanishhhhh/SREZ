package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"srez/internal/objectstorage"
	"srez/internal/storage"
	"srez/internal/usecase"
)

type mediaS3Recorder struct {
	*objectstorage.S3
	puts          atomic.Int64
	failure       bool
	deleteFailure bool
	afterPut      func()
}

func (o *mediaS3Recorder) Put(ctx context.Context, k, mime string, b io.Reader) error {
	o.puts.Add(1)
	if o.failure {
		return errors.New("simulated private S3 failure")
	}
	if e := o.S3.Put(ctx, k, mime, b); e != nil {
		return e
	}
	if o.afterPut != nil {
		o.afterPut()
	}
	return nil
}
func (o *mediaS3Recorder) Delete(ctx context.Context, key string) error {
	if o.deleteFailure {
		return errors.New("S3 delete unavailable")
	}
	return o.S3.Delete(ctx, key)
}
func mediaTestSources(t *testing.T) ([]byte, []byte, []byte) {
	t.Helper()
	var j, p bytes.Buffer
	_ = jpeg.Encode(&j, image.NewRGBA(image.Rect(0, 0, 16, 16)), nil)
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	img.Set(0, 0, color.NRGBA{R: 255, A: 100})
	_ = png.Encode(&p, img)
	w, e := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA")
	if e != nil {
		t.Fatal(e)
	}
	return j.Bytes(), p.Bytes(), w
}
func TestPostgresMediaSeaweedFlow(t *testing.T) {
	endpoint := os.Getenv("TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("TEST_S3_ENDPOINT not set")
	}
	h, db := newPVMHTTP(t)
	ctx := context.Background()
	s3, e := objectstorage.New(objectstorage.Config{Endpoint: endpoint, PublicEndpoint: endpoint, Region: "us-east-1", Bucket: "srez-media", AccessKey: os.Getenv("SREZ_S3_ACCESS_KEY"), SecretKey: os.Getenv("SREZ_S3_SECRET_KEY")})
	if e != nil {
		t.Fatal(e)
	}
	if e = s3.EnsureBucket(ctx); e != nil {
		t.Fatal(e)
	}
	// Positive signing control: prove the same SDK signing/header flow works.
	proofKey, _ := objectstorage.Key("signature-control")
	proof, e := s3.Presign(ctx, proofKey, "image/jpeg")
	if e != nil {
		t.Fatal(e)
	}
	proofRequest, _ := http.NewRequestWithContext(ctx, "PUT", proof.URL, bytes.NewReader([]byte("signature control")))
	for k, v := range proof.Headers {
		proofRequest.Header.Set(k, v)
	}
	proofResponse, e := http.DefaultClient.Do(proofRequest)
	if e != nil {
		t.Fatal("positive signing control failed")
	}
	proofResponse.Body.Close()
	if proofResponse.StatusCode != 200 {
		t.Fatal("positive signing control", proofResponse.StatusCode)
	}
	if e = s3.Delete(ctx, proofKey); e != nil {
		t.Fatal(e)
	}
	retiredKey, _ := objectstorage.Key("staging")
	retired, e := s3.Presign(ctx, retiredKey, "image/jpeg")
	if e != nil {
		t.Fatal(e)
	}
	if e = s3.RetireStaging(ctx); e != nil {
		t.Fatal("cannot revoke legacy uploads", e)
	}
	retiredRequest, _ := http.NewRequestWithContext(ctx, "PUT", retired.URL, bytes.NewReader([]byte("legacy upload")))
	for k, v := range retired.Headers {
		retiredRequest.Header.Set(k, v)
	}
	retiredResponse, e := http.DefaultClient.Do(retiredRequest)
	if e != nil {
		t.Fatal("legacy PUT check failed")
	}
	retiredResponse.Body.Close()
	if retiredResponse.StatusCode != 403 {
		t.Fatal("legacy presigned PUT remains usable", retiredResponse.StatusCode)
	}
	if _, e = s3.Head(ctx, retiredKey); e == nil {
		t.Fatal("revoked legacy PUT stored data")
	}
	objects := &mediaS3Recorder{S3: s3}
	media := usecase.NewMedia(storage.NewMedia(db), objects, "srez-media", h.management)
	h.SetMedia(media)
	owner, ownerID := pvmAccount(t, h, "media_owner")
	editor, editorID := pvmAccount(t, h, "media_editor")
	other, otherID := pvmAccount(t, h, "media_other")
	guest := pvmHTTPClient{t: t, h: h}
	jpegData, pngData, webpData := mediaTestSources(t)
	create := func(c pvmHTTPClient, r usecase.MediaRequest) *usecase.MediaUpload {
		t.Helper()
		body, _ := json.Marshal(r)
		var response struct {
			Upload     usecase.MediaUpload `json:"upload"`
			ContentURL string              `json:"content_url"`
		}
		w := c.call("POST", "/media/uploads", string(body), 201)
		if bytes.Contains(w.Body.Bytes(), []byte("X-Amz")) || bytes.Contains(w.Body.Bytes(), []byte("object_key")) {
			t.Fatal("S3 details leaked")
		}
		if e := json.Unmarshal(w.Body.Bytes(), &response); e != nil {
			t.Fatal(e)
		}
		if e := db.Get(&response.Upload.Asset.ObjectKey, `SELECT object_key FROM media_assets WHERE id=$1`, response.Upload.AssetID); e != nil {
			t.Fatal(e)
		}
		return &response.Upload
	}
	send := func(c pvmHTTPClient, u *usecase.MediaUpload, data []byte, mime string, length int64, want int) *usecase.MediaUpload {
		t.Helper()
		r := httptest.NewRequest("PUT", fmt.Sprintf("/media/uploads/%d/content", u.ID), bytes.NewReader(data))
		r.ContentLength = length
		r.Header.Set("Content-Type", mime)
		if c.token != "" {
			r.Header.Set("Authorization", "Bearer "+c.token)
		}
		w := httptest.NewRecorder()
		h.router.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("upload status %d want %d: %s", w.Code, want, w.Body.String())
		}
		if want != 200 {
			return nil
		}
		result := pvmDecode[usecase.MediaUpload](t, w)
		if e := db.Get(&result.Asset.ObjectKey, `SELECT object_key FROM media_assets WHERE id=$1`, result.AssetID); e != nil {
			t.Fatal(e)
		}
		return &result
	}
	avatarRequest := usecase.MediaRequest{Purpose: "user_avatar", ContentType: "image/jpeg", SizeBytes: int64(len(jpegData))}
	avatar := create(owner, avatarRequest)
	// Pending/staging is never exposed, even to the uploader.
	assetPath := func(id int64) string { return fmt.Sprintf("/media/assets/%d/content", id) }
	owner.call("GET", assetPath(avatar.AssetID), "", 404)
	before := objects.puts.Load()
	send(other, avatar, jpegData, "image/jpeg", int64(len(jpegData)), 404)
	if objects.puts.Load() != before {
		t.Fatal("IDOR reached S3")
	}
	avatar = send(owner, avatar, jpegData, "image/jpeg", int64(len(jpegData)), 200)
	profile := pvmDecode[profileResponse](t, owner.call("GET", "/profile", "", 200))
	if profile.AvatarURL != assetPath(avatar.AssetID) {
		t.Fatal("avatar not bound", profile.AvatarURL)
	}
	response := owner.call("GET", assetPath(avatar.AssetID), "", 200)
	if response.Header().Get("X-Content-Type-Options") != "nosniff" || response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatal("unsafe serving headers")
	}
	anonymous, e := http.Get(endpoint + "/srez-media/" + avatar.Asset.ObjectKey)
	if e != nil {
		t.Fatal(e)
	}
	anonymous.Body.Close()
	if anonymous.StatusCode != 403 {
		t.Fatal("bucket not private", anonymous.StatusCode)
	}
	guest.call("GET", assetPath(avatar.AssetID), "", 404)
	other.call("GET", assetPath(avatar.AssetID), "", 404)
	before = objects.puts.Load()
	send(owner, avatar, jpegData, "image/jpeg", int64(len(jpegData)), 200)
	if before != objects.puts.Load() {
		t.Fatal("replay wrote another object")
	}
	owner.call("POST", fmt.Sprintf("/media/uploads/%d/complete", avatar.ID), `{}`, 410)
	// Concurrent PUTs contend on a durable claim. At most one writes to S3;
	// a later identical attempt may return the already-complete result.
	concurrent := create(owner, avatarRequest)
	start := make(chan struct{})
	statuses := make(chan int, 2)
	parallelBefore := objects.puts.Load()
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			r := httptest.NewRequest("PUT", fmt.Sprintf("/media/uploads/%d/content", concurrent.ID), bytes.NewReader(jpegData))
			r.Header.Set("Authorization", "Bearer "+owner.token)
			r.Header.Set("Content-Type", "image/jpeg")
			w := httptest.NewRecorder()
			h.router.ServeHTTP(w, r)
			statuses <- w.Code
		}()
	}
	close(start)
	success := 0
	for i := 0; i < 2; i++ {
		status := <-statuses
		if status == 200 {
			success++
		} else if status != 409 && status != 429 {
			t.Fatal("concurrent upload status", status)
		}
	}
	if success == 0 || objects.puts.Load() != parallelBefore+1 {
		t.Fatal("duplicate concurrent S3 write")
	}
	avatar = send(owner, concurrent, jpegData, "image/jpeg", int64(len(jpegData)), 200)
	var referenceCount int
	if e = db.Get(&referenceCount, `SELECT count(*) FROM media_references WHERE asset_id=$1`, avatar.AssetID); e != nil || referenceCount != 1 {
		t.Fatal("concurrent binding invariant", e)
	}

	// No alternate URL/mass-assignment path may avoid image validation.
	for _, body := range []string{`{"purpose":"user_avatar","user_id":999}`, `{"purpose":"user_avatar","object_key":"assets/x"}`, `{"purpose":"brand_logo","status":"ready"}`, `{"purpose":"user_avatar","bucket":"other"}`, `{"product_id":1,"color_id":1,"content_type":"image/jpeg","size_bytes":1}`} {
		owner.call("POST", "/media/uploads", body, 400)
	}
	owner.call("PATCH", "/profile", `{"avatar_url":"https://evil.example/x.svg"}`, 400)
	other.call("DELETE", fmt.Sprintf("/users/%d/avatar", ownerID), "", 404)
	for _, kind := range []string{"large-length", "large-actual", "missing-length", "false-length", "corrupt", "wrong-mime", "pixel-bomb"} {
		t.Run(kind, func(t *testing.T) {
			c := owner
			c.t = t
			u := create(c, avatarRequest)
			data := jpegData
			length := int64(len(data))
			mime := "image/jpeg"
			want := 400
			switch kind {
			case "large-length":
				length = usecase.MaxMediaBytes + 1
				want = 413
			case "large-actual":
				data = make([]byte, usecase.MaxMediaBytes+1)
				want = 413
			case "missing-length":
				length = -1
				want = 411
			case "false-length":
				length--
			case "corrupt":
				data = bytes.Repeat([]byte("x"), len(data))
			case "wrong-mime":
				mime = "text/plain"
			case "pixel-bomb":
				var bomb bytes.Buffer
				_ = png.Encode(&bomb, image.NewGray(image.Rect(0, 0, 4000, 4000)))
				r := avatarRequest
				r.ContentType = "image/png"
				r.SizeBytes = int64(bomb.Len())
				u = create(c, r)
				data = bomb.Bytes()
				length = int64(len(data))
				mime = "image/png"
			}
			count := objects.puts.Load()
			send(c, u, data, mime, length, want)
			if objects.puts.Load() != count {
				t.Fatal("invalid file was sent to S3")
			}
			if _, e = s3.Head(ctx, u.Asset.ObjectKey); e == nil {
				t.Fatal("rejected file left an S3 object")
			}
		})
	}
	// Replace avatar with alpha PNG and then WebP; old binding remains until SQL
	// atomically switches, and no source bytes are published.
	for _, source := range []struct {
		data   []byte
		mime   string
		output string
	}{{pngData, "image/png", "image/png"}, {webpData, "image/webp", "image/jpeg"}} {
		r := avatarRequest
		r.ContentType = source.mime
		r.SizeBytes = int64(len(source.data))
		u := create(owner, r)
		u = send(owner, u, source.data, source.mime, int64(len(source.data)), 200)
		if u.Asset.MIME != source.output {
			t.Fatal("output strategy")
		}
		owner.call("GET", assetPath(avatar.AssetID), "", 404)
		avatar = u
	}
	// S3 failure and a real SQL error after S3 PUT preserve the existing avatar.
	failed := create(owner, avatarRequest)
	objects.failure = true
	send(owner, failed, jpegData, "image/jpeg", int64(len(jpegData)), 500)
	objects.failure = false
	profile = pvmDecode[profileResponse](t, owner.call("GET", "/profile", "", 200))
	if profile.AvatarURL != assetPath(avatar.AssetID) {
		t.Fatal("S3 failure lost old avatar")
	}
	failedSQL := create(owner, avatarRequest)
	if _, e = db.Exec(fmt.Sprintf(`CREATE FUNCTION media_fail_ready() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.id=%d AND NEW.state='ready' THEN RAISE EXCEPTION 'private SQL failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER media_fail_ready BEFORE UPDATE ON media_assets FOR EACH ROW EXECUTE FUNCTION media_fail_ready()`, failedSQL.AssetID)); e != nil {
		t.Fatal(e)
	}
	sqlPutCount := objects.puts.Load()
	send(owner, failedSQL, jpegData, "image/jpeg", int64(len(jpegData)), 500)
	if objects.puts.Load() != sqlPutCount+1 {
		t.Fatal("SQL fault did not reach S3", sqlPutCount, objects.puts.Load())
	}
	if _, e = db.Exec(`DROP TRIGGER media_fail_ready ON media_assets; DROP FUNCTION media_fail_ready()`); e != nil {
		t.Fatal(e)
	}
	profile = pvmDecode[profileResponse](t, owner.call("GET", "/profile", "", 200))
	if profile.AvatarURL != assetPath(avatar.AssetID) {
		t.Fatal("SQL rollback lost avatar")
	}
	owner.call("GET", assetPath(failedSQL.AssetID), "", 404)
	if _, e = s3.Head(ctx, failedSQL.Asset.ObjectKey); e != nil {
		t.Fatal("ambiguous candidate unavailable", e)
	}
	// The DB itself rejects cross-owner/cross-purpose bindings.
	if _, e = db.Exec(`UPDATE users SET avatar_url='',avatar_media_id=$1 WHERE id=$2`, avatar.AssetID, otherID); e == nil {
		t.Fatal("DB allowed cross-user asset")
	}
	brand := pvmDecode[brandResponse](t, owner.call("POST", "/brands", `{"name":"Media brand","slug":"media-brand"}`, 201))
	brandPath := fmt.Sprintf("/brands/%d", brand.ID)
	owner.call("POST", brandPath+"/invitations", `{"username":"media_editor"}`, 201)
	editor.call("POST", brandPath+"/invitations/accept", `{}`, 200)
	var logo, banner *usecase.MediaUpload
	for _, purpose := range []string{"brand_logo", "brand_banner"} {
		r := usecase.MediaRequest{Purpose: purpose, BrandID: brand.ID, ContentType: "image/jpeg", SizeBytes: int64(len(jpegData))}
		body, _ := json.Marshal(r)
		other.call("POST", "/media/uploads", string(body), 404)
		editor.call("POST", "/media/uploads", string(body), 403)
		u := create(owner, r)
		u = send(owner, u, jpegData, "image/jpeg", int64(len(jpegData)), 200)
		guest.call("GET", assetPath(u.AssetID), "", 404)
		editor.call("GET", assetPath(u.AssetID), "", 200)
		replacement := create(owner, r)
		replacement = send(owner, replacement, jpegData, "image/jpeg", int64(len(jpegData)), 200)
		owner.call("GET", assetPath(u.AssetID), "", 404)
		if purpose == "brand_logo" {
			logo = replacement
		} else {
			banner = replacement
		}
	}
	owner.call("PATCH", brandPath, `{"logo_url":"https://evil.example/x.svg"}`, 400)
	owner.call("PATCH", brandPath, `{"description":"Complete media brand"}`, 200)
	owner.call("POST", brandPath+"/publish", `{}`, 200)
	guest.call("GET", assetPath(logo.AssetID), "", 200)
	guest.call("GET", assetPath(banner.AssetID), "", 200)
	owner.call("DELETE", brandPath+"/media/brand_logo", "", 409) // active brand still requires logo
	owner.call("DELETE", brandPath+"/media/brand_banner", "", 204)
	guest.call("GET", assetPath(banner.AssetID), "", 404)
	product := pvmDecode[productResponse](t, owner.call("POST", "/products", fmt.Sprintf(`{"brand_id":%d,"name":"Media product","description":"Complete product","price_kopecks":125000,"category":"tshirt","materials":["cotton"]}`, brand.ID), 201))
	productPath := fmt.Sprintf("/products/%d", product.ID)
	colorResponse := pvmDecode[managedColorResponse](t, editor.call("POST", productPath+"/colors", `{"color":"Black"}`, 201))
	colorPath := fmt.Sprintf("%s/colors/%d", productPath, colorResponse.ID)
	editor.call("POST", colorPath+"/variants", `{"size":"M","stock":10}`, 201)
	imageRequest := usecase.MediaRequest{Purpose: "product_image", ProductID: product.ID, ColorID: colorResponse.ID, ContentType: "image/jpeg", SizeBytes: int64(len(jpegData))}
	front := create(editor, imageRequest)
	front = send(editor, front, jpegData, "image/jpeg", int64(len(jpegData)), 200)
	if front.ImageID == nil {
		t.Fatal("product upload missing stable image_id")
	}
	back := create(owner, imageRequest)
	back = send(owner, back, jpegData, "image/jpeg", int64(len(jpegData)), 200)
	editor.call("POST", colorPath+"/images", `{"url":"https://evil.example/image.png"}`, 400)
	editor.call("POST", colorPath+"/images", fmt.Sprintf(`{"media_id":%d}`, front.AssetID), 200)
	owner.call("POST", productPath+"/publish", `{}`, 200)
	guest.call("GET", assetPath(front.AssetID), "", 200)
	detail := pvmDecode[productDetailResponse](t, guest.call("GET", productPath, "", 200))
	if len(detail.Colors[0].Images) != 2 || detail.Colors[0].Images[0] != assetPath(front.AssetID) {
		t.Fatal("managed public DTO")
	}
	images := pvmDecode[[]managedImageResponse](t, editor.call("POST", colorPath+"/images/reorder", fmt.Sprintf(`{"image_ids":[%d,%d]}`, *back.ImageID, *front.ImageID), 200))
	if images[0].ID != *back.ImageID || images[0].Position != 0 {
		t.Fatal("managed reorder")
	}
	otherProduct := pvmDecode[productResponse](t, owner.call("POST", "/products", fmt.Sprintf(`{"brand_id":%d,"name":"Other draft"}`, brand.ID), 201))
	otherPath := fmt.Sprintf("/products/%d", otherProduct.ID)
	otherColor := pvmDecode[managedColorResponse](t, owner.call("POST", otherPath+"/colors", `{"color":"Other"}`, 201))
	otherColorPath := fmt.Sprintf("%s/colors/%d", otherPath, otherColor.ID)
	owner.call("POST", otherColorPath+"/images", fmt.Sprintf(`{"media_id":%d}`, front.AssetID), 404)
	wrong := imageRequest
	wrong.ColorID = otherColor.ID
	body, _ := json.Marshal(wrong)
	owner.call("POST", "/media/uploads", string(body), 404)
	foreignBrand := pvmDecode[brandResponse](t, other.call("POST", "/brands", `{"name":"Foreign media brand","slug":"foreign-media-brand"}`, 201))
	foreignProduct := pvmDecode[productResponse](t, other.call("POST", "/products", fmt.Sprintf(`{"brand_id":%d,"name":"Foreign product"}`, foreignBrand.ID), 201))
	foreignPath := fmt.Sprintf("/products/%d", foreignProduct.ID)
	foreignColor := pvmDecode[managedColorResponse](t, other.call("POST", foreignPath+"/colors", `{"color":"Foreign"}`, 201))
	wrong = imageRequest
	wrong.ProductID = foreignProduct.ID
	wrong.ColorID = foreignColor.ID
	body, _ = json.Marshal(wrong)
	owner.call("POST", "/media/uploads", string(body), 404)
	// Revoke DURING S3 I/O. Final parent transaction must observe revoked rights.
	revoked := create(editor, imageRequest)
	objects.afterPut = func() {
		if e := h.management.Revoke(ctx, ownerID, brand.ID, editorID); e != nil {
			t.Fatal(e)
		}
	}
	send(editor, revoked, jpegData, "image/jpeg", int64(len(jpegData)), 404)
	objects.afterPut = nil
	owner.call("GET", assetPath(revoked.AssetID), "", 404)
	body, _ = json.Marshal(imageRequest)
	editor.call("POST", "/media/uploads", string(body), 404)
	owner.call("DELETE", fmt.Sprintf("%s/images/%d", colorPath, *front.ImageID), "", 204)
	owner.call("DELETE", fmt.Sprintf("%s/images/%d", colorPath, *back.ImageID), "", 409)
	var state string
	if e = db.Get(&state, `SELECT state FROM media_assets WHERE id=$1`, back.AssetID); e != nil || state != "ready" {
		t.Fatal("failed last-image deletion queued live asset", e)
	}
	replaceRequest := imageRequest
	replaceRequest.ImageID = back.ImageID
	replacement := create(owner, replaceRequest)
	replacement = send(owner, replacement, jpegData, "image/jpeg", int64(len(jpegData)), 200)
	if *replacement.ImageID != *back.ImageID {
		t.Fatal("replacement changed image_id")
	}
	guest.call("GET", assetPath(back.AssetID), "", 404)
	// Replaying a previous complete upload cannot restore an old image binding.
	send(owner, back, jpegData, "image/jpeg", int64(len(jpegData)), 200)
	guest.call("GET", assetPath(replacement.AssetID), "", 200)
	owner.call("POST", productPath+"/hide", `{}`, 200)
	guest.call("GET", assetPath(replacement.AssetID), "", 404)
	owner.call("DELETE", colorPath, "", 204)
	owner.call("DELETE", "/profile/avatar", "", 204)
	owner.call("GET", assetPath(avatar.AssetID), "", 404)
	// Deterministic expiry for cleanup, with source/object keys kept in SQL.
	if _, e = db.Exec(`UPDATE media_assets SET expires_at=now()-interval '1 hour',delete_after=now()-interval '1 hour' WHERE state<>'ready'`); e != nil {
		t.Fatal(e)
	}
	objects.deleteFailure = true
	if e = media.Cleanup(ctx); e == nil {
		t.Fatal("S3 cleanup failure ignored")
	}
	if _, e = s3.Head(ctx, revoked.Asset.ObjectKey); e != nil {
		t.Fatal("failed delete lost candidate", e)
	}
	objects.deleteFailure = false
	if e = media.Cleanup(ctx); e != nil {
		t.Fatal(e)
	}
	for _, u := range []*usecase.MediaUpload{avatar, failedSQL, revoked, replacement} {
		if _, e = s3.Head(ctx, u.Asset.ObjectKey); e == nil {
			t.Fatal("unreferenced candidate survived GC", u.AssetID)
		}
	}
	if _, e = s3.Head(ctx, logo.Asset.ObjectKey); e != nil {
		t.Fatal("cleanup deleted live logo", e)
	}
	t.Log("PASS: backend JPEG/PNG/WebP; strict pre-S3 limits; avatar, brand, product bindings; real SQL rollback; revoke during S3; BOLA; replay; reorder; publication guards; private delivery; GC")
}
