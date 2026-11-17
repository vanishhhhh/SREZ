package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"srez/internal/devseed"
	"srez/internal/entities"
	"srez/internal/security"
	"srez/internal/storage"
	"srez/internal/usecase"
)

// Never print capability URLs even if a request assertion fails.
func wishlistCall(t *testing.T, c pvmHTTPClient, method, path, body string, want int) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	w := httptest.NewRecorder()
	c.h.router.ServeHTTP(w, req)
	if w.Code != want {
		safe := path
		if strings.HasPrefix(path, "/wishlists/shared/") {
			safe = "/wishlists/shared/[redacted]"
		}
		t.Fatalf("%s %s status=%d want=%d", method, safe, w.Code, want)
	}
	return w
}
func TestPostgresHTTPWishlists(t *testing.T) {
	h, db := newPVMHTTP(t)
	links, _ := security.NewWishlistLinks(bytes.Repeat([]byte{1}, 32))
	service := usecase.NewWishlists(storage.NewWishlists(db), links)
	h.SetWishlists(service)
	owner, _ := pvmAccount(t, h, "wishlist_owner")
	other, _ := pvmAccount(t, h, "wishlist_other")
	guest := pvmHTTPClient{t: t, h: h}
	call := func(c pvmHTTPClient, m, p, b string, status int) *httptest.ResponseRecorder {
		return wishlistCall(t, c, m, p, b, status)
	}
	if err := devseed.Apply(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	products := pvmDecode[[]productCardResponse](t, owner.call("GET", "/products", "", 200))
	p1, p2 := products[0], products[1]
	d1, _ := storage.NewProducts(db).GetPublic(context.Background(), p1.ID)
	d2, _ := storage.NewProducts(db).GetPublic(context.Background(), p2.ID)
	v1, v2 := d1.Publication.Variants[0].ID(), d2.Publication.Variants[0].ID()
	call(guest, "GET", "/wishlists", "", 401)
	call(guest, "GET", "/wishlists/shared/not-a-valid-token", "", 404)
	for _, body := range []string{`{}`, `{"title":null}`, `{"title":"x","owner_user_id":1}`, `{"title":"x","visibility":"public"}`, `{"title":"x","share_token":"bad"}`, `{"title":"x"} {}`} {
		call(owner, "POST", "/wishlists", body, 400)
	}
	w := pvmDecode[wishlistResponse](t, call(owner, "POST", "/wishlists", `{"title":"  Мои хотелки  ","description":"Осень"}`, 201))
	path := fmt.Sprintf("/wishlists/%d", w.ID)
	if w.Title != "Мои хотелки" || w.Visibility != "private" || w.SharePath != nil || len(w.Items) != 0 {
		t.Fatal("new private list")
	}
	for _, m := range []string{"GET", "PATCH", "DELETE"} {
		body := ""
		if m == "PATCH" {
			body = `{"title":"stolen"}`
		}
		call(other, m, path, body, 404)
	}
	call(other, "POST", path+"/share", `{}`, 404)
	call(guest, "POST", path+"/items", fmt.Sprintf(`{"product_id":%d}`, p1.ID), 401)
	call(owner, "PATCH", path, `{"description":null}`, 400)
	call(owner, "PATCH", path, `{"owner_user_id":2}`, 400)
	call(owner, "PATCH", path, `{"title":"Зимняя подборка","description":"Своё видение"}`, 200)
	call(owner, "POST", path+"/items", fmt.Sprintf(`{"product_id":%d,"product_variant_id":%d}`, p1.ID, v2), 404)
	w = pvmDecode[wishlistResponse](t, call(owner, "POST", path+"/items", fmt.Sprintf(`{"product_id":%d}`, p1.ID), 201))
	i1 := w.Items[0].ID
	call(owner, "POST", path+"/items", fmt.Sprintf(`{"product_id":%d}`, p1.ID), 409)
	w = pvmDecode[wishlistResponse](t, call(owner, "POST", path+"/items", fmt.Sprintf(`{"product_id":%d,"product_variant_id":%d}`, p2.ID, v2), 201))
	i2 := w.Items[1].ID
	ip := fmt.Sprintf("%s/items/%d", path, i1)
	call(other, "PATCH", ip, `{"product_variant_id":null}`, 404)
	call(owner, "PATCH", ip, `{}`, 400)
	w = pvmDecode[wishlistResponse](t, call(owner, "PATCH", ip, fmt.Sprintf(`{"product_variant_id":%d}`, v1), 200))
	if w.Items[0].Variant == nil || w.Items[0].Variant.ID != v1 {
		t.Fatal("variant selection")
	}
	call(owner, "PATCH", ip, `{"product_variant_id":null}`, 200)
	w = pvmDecode[wishlistResponse](t, call(owner, "POST", path+"/items/reorder", fmt.Sprintf(`{"item_ids":[%d,%d]}`, i2, i1), 200))
	if w.Items[0].ID != i2 || w.Items[0].Position != 0 || w.Items[1].Position != 1 {
		t.Fatal("non-atomic reorder")
	}
	call(owner, "POST", path+"/items/reorder", fmt.Sprintf(`{"item_ids":[%d,%d]}`, i1, i1), 400)
	call(owner, "POST", path+"/items/reorder", fmt.Sprintf(`{"item_ids":[%d]}`, i1), 409)
	call(other, "POST", path+"/items/reorder", `{"item_ids":[]}`, 404)
	w = pvmDecode[wishlistResponse](t, call(owner, "POST", path+"/share", `{}`, 200))
	if w.SharePath == nil {
		t.Fatal("missing share path")
	}
	token := strings.TrimPrefix(*w.SharePath, "/w/")
	shared := "/wishlists/shared/" + token
	public := call(guest, "GET", shared, "", 200)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(public.Body.Bytes(), &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 4 || fields["title"] == nil || fields["description"] == nil || fields["updated_at"] == nil || fields["items"] == nil {
		t.Fatal("public response leaked extra fields")
	}
	if public.Header().Get("Cache-Control") != "no-store" || public.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("public caching/referrer policy")
	}
	stable := pvmDecode[wishlistResponse](t, call(owner, "GET", path, "", 200))
	if stable.SharePath == nil || *stable.SharePath != *w.SharePath {
		t.Fatal("owner link lost after reload")
	}
	var hash, sealed []byte
	if err := db.QueryRow(`SELECT share_token_hash,share_token_sealed FROM wishlists WHERE id=$1`, w.ID).Scan(&hash, &sealed); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(hash, security.Digest(token)) || bytes.Contains(sealed, []byte(token)) {
		t.Fatal("token is not protected at rest")
	}
	w = pvmDecode[wishlistResponse](t, call(owner, "POST", path+"/share", `{}`, 200))
	if w.SharePath == nil || *w.SharePath == "/w/"+token {
		t.Fatal("link not rotated")
	}
	call(guest, "GET", shared, "", 404)
	shared = "/wishlists/shared/" + strings.TrimPrefix(*w.SharePath, "/w/")
	call(guest, "GET", shared, "", 200)
	call(owner, "DELETE", path+"/share", "", 200)
	call(guest, "GET", shared, "", 404)
	call(owner, "DELETE", path+"/share", "", 200)
	w = pvmDecode[wishlistResponse](t, call(owner, "POST", path+"/share", `{}`, 200))
	shared = "/wishlists/shared/" + strings.TrimPrefix(*w.SharePath, "/w/")
	if _, err := db.Exec(`UPDATE products SET status='hidden' WHERE id=$1`, p1.ID); err != nil {
		t.Fatal(err)
	}
	pub := pvmDecode[struct {
		Items []wishlistItemResponse `json:"items"`
	}](t, call(guest, "GET", shared, "", 200))
	if len(pub.Items) != 1 || pub.Items[0].ProductID != p2.ID || pub.Items[0].Position != 0 {
		t.Fatal("hidden product exposed")
	}
	private := pvmDecode[wishlistResponse](t, call(owner, "GET", path, "", 200))
	if len(private.Items) != 2 || private.Items[1].Product != nil {
		t.Fatal("owner tombstone lost")
	}
	if _, err := db.Exec(`UPDATE brands SET status='suspended' WHERE id=$1`, p2.BrandID); err != nil {
		t.Fatal(err)
	}
	pub = pvmDecode[struct {
		Items []wishlistItemResponse `json:"items"`
	}](t, call(guest, "GET", shared, "", 200))
	if len(pub.Items) != 0 {
		t.Fatal("inactive brand exposed")
	}
	call(owner, "DELETE", ip, "", 200)
	call(owner, "DELETE", path, "", 204)
	call(guest, "GET", shared, "", 404)
	for _, id := range []string{"0", "-1", "+1", "01", "1.2", "9223372036854775808"} {
		call(owner, "GET", "/wishlists/"+id, "", 400)
	}
}

func TestPostgresWishlistConcurrentLimitsAndReorder(t *testing.T) {
	h, db := newPVMHTTP(t)
	links, _ := security.NewWishlistLinks(bytes.Repeat([]byte{2}, 32))
	service := usecase.NewWishlists(storage.NewWishlists(db), links)
	h.SetWishlists(service)
	_, uid := pvmAccount(t, h, "wishlist_limits")
	ctx := context.Background()
	var wg sync.WaitGroup
	results := make(chan error, 25)
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := service.Create(ctx, uid, "Мой список", ""); results <- err }()
	}
	wg.Wait()
	close(results)
	created, limited := 0, 0
	for err := range results {
		if err == nil {
			created++
		} else if errors.Is(err, usecase.ErrWishlistLimit) {
			limited++
		} else {
			t.Fatal(err)
		}
	}
	if created != entities.MaxWishlists || limited != 5 {
		t.Fatal("owner limit raced")
	}
	if err := devseed.Apply(ctx, db); err != nil {
		t.Fatal(err)
	}
	lists, err := service.List(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	list := lists[0].ID
	var products []int64
	if err = db.Select(&products, `SELECT id FROM products WHERE status='published' ORDER BY id LIMIT 3`); err != nil {
		t.Fatal(err)
	}
	for _, pid := range products {
		if _, err = service.AddItem(ctx, uid, list, pid, nil); err != nil {
			t.Fatal(err)
		}
	}
	d, _ := service.Detail(ctx, uid, list)
	ids := []int64{d.Items[1].ID, d.Items[0].ID}
	race := make(chan error, 2)
	go func() { _, err := service.Reorder(ctx, uid, list, ids); race <- err }()
	go func() { race <- service.RemoveItem(ctx, uid, list, ids[1]) }()
	for i := 0; i < 2; i++ {
		if err = <-race; err != nil && !errors.Is(err, usecase.ErrWishlistChanged) {
			t.Fatal(err)
		}
	}
	d, err = service.Detail(ctx, uid, list)
	if err != nil || len(d.Items) != 1 || d.Items[0].Position != 0 {
		t.Fatal("reorder/remove invariant")
	}
	// Same-product insert races produce one entry and a single explicit conflict.
	list = lists[1].ID
	race = make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { _, err := service.AddItem(ctx, uid, list, products[0], nil); race <- err }()
	}
	ok, duplicate := 0, 0
	for i := 0; i < 2; i++ {
		err = <-race
		if err == nil {
			ok++
		} else if errors.Is(err, usecase.ErrWishlistDuplicate) {
			duplicate++
		} else {
			t.Fatal(err)
		}
	}
	if ok != 1 || duplicate != 1 {
		t.Fatal("duplicate race")
	}
}

func TestPostgresWishlistVariantIntegrityAndDeletion(t *testing.T) {
	h, db := newPVMHTTP(t)
	links, _ := security.NewWishlistLinks(bytes.Repeat([]byte{3}, 32))
	s := usecase.NewWishlists(storage.NewWishlists(db), links)
	h.SetWishlists(s)
	_, uid := pvmAccount(t, h, "wishlist_fk")
	ctx := context.Background()
	if err := devseed.Apply(ctx, db); err != nil {
		t.Fatal(err)
	}
	var variants []struct {
		ID        int64 `db:"id"`
		ProductID int64 `db:"product_id"`
	}
	if err := db.Select(&variants, `SELECT DISTINCT ON(c.product_id) v.id,c.product_id FROM product_variants v JOIN product_colors c ON c.id=v.product_color_id ORDER BY c.product_id,v.id LIMIT 2`); err != nil {
		t.Fatal(err)
	}
	w, err := s.Create(ctx, uid, "Размеры", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE wishlists SET visibility='public',share_token_hash=NULL,share_token_sealed=$2 WHERE id=$1`, w.Wishlist.ID, make([]byte, 32)); err == nil {
		t.Fatal("SQL UNKNOWN bypassed public-link integrity")
	}
	first, foreign := variants[0], variants[1]
	w, err = s.AddItem(ctx, uid, w.Wishlist.ID, first.ProductID, &first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE wishlist_items SET product_variant_id=$2 WHERE id=$1`, w.Items[0].ID, foreign.ID); err == nil {
		t.Fatal("database accepted a foreign variant")
	}
	if _, err = db.Exec(`DELETE FROM product_variants WHERE id=$1`, first.ID); err != nil {
		t.Fatal(err)
	}
	w, err = s.Detail(ctx, uid, w.Wishlist.ID)
	if err != nil || w.Items[0].VariantID != nil || w.Items[0].Variant != nil {
		t.Fatal("deleted variant did not detach safely")
	}
}

func TestPostgresWishlistItemLimitIsAtomic(t *testing.T) {
	h, db := newPVMHTTP(t)
	links, _ := security.NewWishlistLinks(bytes.Repeat([]byte{4}, 32))
	s := usecase.NewWishlists(storage.NewWishlists(db), links)
	h.SetWishlists(s)
	_, uid := pvmAccount(t, h, "wishlist_item_limit")
	ctx := context.Background()
	m := usecase.NewManagement(storage.NewManagement(db))
	brand, err := m.CreateBrand(ctx, uid, "Limits Brand", "wishlist-limits-brand")
	if err != nil {
		t.Fatal(err)
	}
	// Complete public catalog fixtures, exclusively inside this test schema.
	_, err = db.Exec(`UPDATE brands SET status='active',description='Limits fixture',logo_url='https://example.com/logo.jpg' WHERE id=$1`, brand.ID())
	if err != nil {
		t.Fatal(err)
	}
	var products []int64
	err = db.Select(&products, `INSERT INTO products(brand_id,name,description,price_kopecks,category,materials,status,created_at,updated_at)
 SELECT $1,'Wishlist fixture '||n,'Limits fixture',10000,'hoodie',ARRAY['cotton'],'published',now(),now() FROM generate_series(1,201) n RETURNING id`, brand.ID())
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO product_colors(product_id,color,color_key) SELECT id,'Black','black' FROM products WHERE brand_id=$1`, brand.ID())
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO product_variants(product_color_id,size,size_key,stock,sku) SELECT c.id,'M','m',1,'' FROM product_colors c JOIN products p ON p.id=c.product_id WHERE p.brand_id=$1`, brand.ID())
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO product_color_images(product_color_id,url,position) SELECT c.id,'https://example.com/front.jpg',0 FROM product_colors c JOIN products p ON p.id=c.product_id WHERE p.brand_id=$1`, brand.ID())
	if err != nil {
		t.Fatal(err)
	}
	w, err := s.Create(ctx, uid, "Двести вещей", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO wishlist_items(wishlist_id,product_id,position) SELECT $1,id,(row_number() OVER(ORDER BY id)-1)::integer FROM products WHERE brand_id=$2 ORDER BY id LIMIT 199`, w.Wishlist.ID, brand.ID())
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	for _, product := range products[199:] {
		go func(pid int64) { _, e := s.AddItem(ctx, uid, w.Wishlist.ID, pid, nil); results <- e }(product)
	}
	added, limited := 0, 0
	for i := 0; i < 2; i++ {
		e := <-results
		if e == nil {
			added++
		} else if errors.Is(e, usecase.ErrWishlistLimit) {
			limited++
		} else {
			t.Fatal(e)
		}
	}
	if added != 1 || limited != 1 {
		t.Fatal("item limit raced")
	}
	d, err := s.Detail(ctx, uid, w.Wishlist.ID)
	if err != nil || len(d.Items) != entities.MaxWishlistItems || d.Items[199].Position != 199 {
		t.Fatal("item limit/order invalid")
	}
}
