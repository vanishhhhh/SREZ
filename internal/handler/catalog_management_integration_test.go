package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"srez/internal/config"
	"srez/internal/security"
	"srez/internal/storage"
	"srez/internal/usecase"
	"srez/migrations"
)

func TestPostgresHTTPAuthManagementCatalogIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	admin, e := sqlx.Connect("postgres", dsn)
	if e != nil {
		t.Fatal(e)
	}
	schema := fmt.Sprintf("http_catalog_test_%d", time.Now().UnixNano())
	if _, e = admin.Exec(`CREATE SCHEMA ` + pq.QuoteIdentifier(schema)); e != nil {
		admin.Close()
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if _, e := admin.Exec(`DROP SCHEMA ` + pq.QuoteIdentifier(schema) + ` CASCADE`); e != nil {
			t.Error(e)
		}
		admin.Close()
	})
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
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	if e = migrations.Apply(ctx, db); e != nil {
		t.Fatal(e)
	}
	tokens, e := security.NewTokens(security.Digest(security.Random()), security.Digest(security.Random()), "srez", "api")
	if e != nil {
		t.Fatal(e)
	}
	brands := storage.NewBrands(db)
	h := New(usecase.NewUsers(storage.NewUsers(db)), usecase.NewBrands(brands), usecase.NewProducts(storage.NewProducts(db), brands))
	h.SetManagement(usecase.NewManagement(storage.NewManagement(db)))
	if e = h.EnableAuth(usecase.NewAuth(storage.NewAuth(db), security.NewPasswords(2), tokens), tokens, config.Auth{Origin: "http://127.0.0.1:8080", LocalHTTP: true, Attempts: 20}); e != nil {
		t.Fatal(e)
	}
	call := func(method, path, body, access, csrf string, cookie *http.Cookie, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if method != "GET" {
			req.Header.Set("Origin", "http://127.0.0.1:8080")
		} else {
			req.Header.Set("Sec-Fetch-Site", "same-origin")
		}
		if access != "" {
			req.Header.Set("Authorization", "Bearer "+access)
		}
		if csrf != "" {
			req.Header.Set("X-CSRF-Token", csrf)
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.router.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s %s status=%d want=%d", method, path, w.Code, want)
		}
		return w
	}
	decode := func(w *httptest.ResponseRecorder, target any) {
		t.Helper()
		if e := json.Unmarshal(w.Body.Bytes(), target); e != nil {
			t.Fatal(e)
		}
	}
	registerLogin := func(name string) (publicUserResponse, usecase.AuthResult, *http.Cookie) {
		t.Helper()
		body := fmt.Sprintf(`{"email":"%s@example.com","username":"%s","display_name":"%s","password":"cross feature test password"}`, name, name, name)
		var user publicUserResponse
		decode(call("POST", "/auth/register", body, "", "", nil, 201), &user)
		w := call("POST", "/auth/login", fmt.Sprintf(`{"identifier":"%s","password":"cross feature test password"}`, name), "", "", nil, 200)
		var login usecase.AuthResult
		decode(w, &login)
		cookies := w.Result().Cookies()
		if len(cookies) != 1 {
			t.Fatal("missing refresh cookie")
		}
		return user, login, cookies[0]
	}
	owner, login, cookie := registerLogin("cross_owner")
	editor, editorLogin, _ := registerLogin("cross_editor")
	profile := call("GET", "/profile", "", login.AccessToken, "", nil, 200)
	var ownProfile profileResponse
	decode(profile, &ownProfile)
	if ownProfile.ID != owner.ID {
		t.Fatal("wrong authenticated profile")
	}
	var csrf struct {
		Token string `json:"csrf_token"`
	}
	decode(call("GET", "/auth/csrf", "", "", "", cookie, 200), &csrf)
	if csrf.Token != login.CSRFToken {
		t.Fatal("csrf mismatch")
	}
	call("POST", "/auth/refresh", "{}", "", "", cookie, 403)
	rotatedResponse := call("POST", "/auth/refresh", "{}", "", csrf.Token, cookie, 200)
	var rotated usecase.AuthResult
	decode(rotatedResponse, &rotated)
	cookie = rotatedResponse.Result().Cookies()[0]
	call("POST", "/products", `{"brand_id":1,"name":"Denied"}`, "", "", nil, 401)
	var brand brandResponse
	decode(call("POST", "/brands", `{"name":"Cross Brand","slug":"cross-brand"}`, rotated.AccessToken, "", nil, 201), &brand)
	bid := strconv.FormatInt(brand.ID, 10)
	call("PATCH", "/brands/"+bid, `{"description":"Complete brand","logo_url":"https://example.com/logo.png"}`, rotated.AccessToken, "", nil, 200)
	call("POST", "/brands/"+bid+"/publish", "{}", rotated.AccessToken, "", nil, 200)
	call("POST", "/brands/"+bid+"/invitations", `{"username":"cross_editor"}`, rotated.AccessToken, "", nil, 201)
	call("POST", "/brands/"+bid+"/invitations/accept", "{}", editorLogin.AccessToken, "", nil, 200)
	var product productResponse
	decode(call("POST", "/products", fmt.Sprintf(`{"brand_id":%d,"name":"Cross Product","description":"Complete product","price_kopecks":125000,"category":"tshirt","materials":["cotton"]}`, brand.ID), editorLogin.AccessToken, "", nil, 201), &product)
	pid := strconv.FormatInt(product.ID, 10)
	call("GET", "/products/"+pid, "", "", "", nil, 404)
	call("POST", "/products/"+pid+"/publish", "{}", rotated.AccessToken, "", nil, 409)
	// Trusted fixtures only in the isolated test schema: no router bypass.
	for _, c := range []struct {
		name, key string
		sizes     []string
		stocks    []int
	}{{"Black", "black", []string{"M", "L"}, []int{3, 0}}, {"White", "white", []string{"One Size"}, []int{5}}} {
		var id int64
		if e = db.Get(&id, `INSERT INTO product_colors(product_id,color,color_key) VALUES($1,$2,$3) RETURNING id`, product.ID, c.name, c.key); e != nil {
			t.Fatal(e)
		}
		for pos, suffix := range []string{"front", "back"} {
			if _, e = db.Exec(`INSERT INTO product_color_images(product_color_id,position,url) VALUES($1,$2,$3)`, id, pos, "https://example.com/"+c.key+"-"+suffix+".png"); e != nil {
				t.Fatal(e)
			}
		}
		for i, size := range c.sizes {
			if _, e = db.Exec(`INSERT INTO product_variants(product_color_id,size,size_key,stock,sku) VALUES($1,$2,$3,$4,'CROSS')`, id, size, strings.ToLower(size), c.stocks[i]); e != nil {
				t.Fatal(e)
			}
		}
	}
	call("POST", "/products/"+pid+"/publish", "{}", rotated.AccessToken, "", nil, 200)
	browse := call("GET", "/products?brand_id="+bid+"&category=tshirt&min_price=125000&max_price=125000&color=black&size=M&sort=price_desc&page=1&limit=1", "", "", "", nil, 200)
	for header, want := range map[string]string{"X-Total-Count": "1", "X-Page": "1", "X-Limit": "1", "X-Total-Pages": "1"} {
		if browse.Header().Get(header) != want {
			t.Fatalf("wrong %s", header)
		}
	}
	var cards []productCardResponse
	decode(browse, &cards)
	if len(cards) != 1 || cards[0].ID != product.ID || cards[0].BrandName != brand.Name || cards[0].BrandSlug != brand.Slug || cards[0].PreviewImageURL != "https://example.com/black-front.png" || !cards[0].IsAvailable || len(cards[0].Colors) != 2 {
		t.Fatal("catalog card lost cross-feature data")
	}
	if strings.Contains(strings.Join(cards[0].AvailableSizes, ","), "L") {
		t.Fatal("sold-out size exposed as available")
	}
	for _, suffix := range []string{"&color=black&size=One+Size", "&size=L", "&page=2&limit=1"} {
		var items []productCardResponse
		decode(call("GET", "/products?brand_id="+bid+suffix, "", "", "", nil, 200), &items)
		if len(items) != 0 {
			t.Fatal("filter/page mismatch")
		}
	}
	var detail productDetailResponse
	decode(call("GET", "/products/"+pid, "", "", "", nil, 200), &detail)
	if len(detail.Colors) != 2 || len(detail.Colors[0].Images) != 2 || len(detail.Colors[0].Variants) != 2 || detail.Colors[0].Variants[1].Stock != 0 {
		t.Fatal("detail contract changed")
	}
	call("DELETE", "/brands/"+bid+"/members/"+strconv.FormatInt(editor.ID, 10), "", rotated.AccessToken, "", nil, 204)
	call("PATCH", "/products/"+pid, `{"name":"Denied after revoke"}`, editorLogin.AccessToken, "", nil, 404)
	call("POST", "/products/"+pid+"/hide", "{}", rotated.AccessToken, "", nil, 200)
	var hidden []productCardResponse
	decode(call("GET", "/products?brand_id="+bid, "", "", "", nil, 200), &hidden)
	if len(hidden) != 0 {
		t.Fatal("hidden product leaked")
	}
	call("POST", "/auth/logout", "{}", "", rotated.CSRFToken, cookie, 204)
	call("POST", "/auth/refresh", "{}", "", rotated.CSRFToken, cookie, 401)
}
