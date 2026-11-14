package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
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

type pvmHTTPClient struct {
	t     *testing.T
	h     *Handler
	token string
}

func (c pvmHTTPClient) call(method, path, body string, want int) *httptest.ResponseRecorder {
	c.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://127.0.0.1:8080")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	w := httptest.NewRecorder()
	c.h.router.ServeHTTP(w, req)
	if w.Code != want {
		c.t.Fatalf("%s %s: status=%d want=%d", method, path, w.Code, want)
	}
	return w
}
func pvmDecode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var result T
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}
func newPVMHTTP(t *testing.T) (*Handler, *sqlx.DB) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	admin, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("pvm_http_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec(`CREATE SCHEMA ` + pq.QuoteIdentifier(schema)); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(`DROP SCHEMA ` + pq.QuoteIdentifier(schema) + ` CASCADE`); err != nil {
			t.Error(err)
		}
		admin.Close()
	})
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := sqlx.Connect("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err = migrations.Apply(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	tokens, err := security.NewTokens(security.Digest(security.Random()), security.Digest(security.Random()), "srez", "api")
	if err != nil {
		t.Fatal(err)
	}
	brands := storage.NewBrands(db)
	h := New(usecase.NewUsers(storage.NewUsers(db)), usecase.NewBrands(brands), usecase.NewProducts(storage.NewProducts(db), brands))
	h.SetManagement(usecase.NewManagement(storage.NewManagement(db)))
	if err = h.EnableAuth(usecase.NewAuth(storage.NewAuth(db), security.NewPasswords(2), tokens), tokens, config.Auth{Origin: "http://127.0.0.1:8080", LocalHTTP: true, Attempts: 50}); err != nil {
		t.Fatal(err)
	}
	return h, db
}
func pvmAccount(t *testing.T, h *Handler, name string) (pvmHTTPClient, int64) {
	t.Helper()
	guest := pvmHTTPClient{t: t, h: h}
	user := pvmDecode[publicUserResponse](t, guest.call("POST", "/auth/register", fmt.Sprintf(`{"email":"%s@example.com","username":"%s","display_name":"%s","password":"local PVM test password 123"}`, name, name, name), 201))
	login := pvmDecode[usecase.AuthResult](t, guest.call("POST", "/auth/login", fmt.Sprintf(`{"identifier":"%s","password":"local PVM test password 123"}`, name), 200))
	return pvmHTTPClient{t: t, h: h, token: login.AccessToken}, user.ID
}

func TestPostgresHTTPProductVariantManagement(t *testing.T) {
	h, _ := newPVMHTTP(t)
	owner, _ := pvmAccount(t, h, "pvm_owner")
	editor, editorID := pvmAccount(t, h, "pvm_editor")
	outsider, _ := pvmAccount(t, h, "pvm_outsider")
	invited, _ := pvmAccount(t, h, "pvm_invited")
	brand := pvmDecode[brandResponse](t, owner.call("POST", "/brands", `{"name":"PVM Brand","slug":"pvm-brand"}`, 201))
	bpath := fmt.Sprintf("/brands/%d", brand.ID)
	owner.call("POST", bpath+"/invitations", `{"username":"pvm_editor"}`, 201)
	owner.call("POST", bpath+"/invitations", `{"username":"pvm_invited"}`, 201)
	editor.call("POST", bpath+"/invitations/accept", `{}`, 200)
	product := pvmDecode[productResponse](t, editor.call("POST", "/products", fmt.Sprintf(`{"brand_id":%d,"name":"PVM Hoodie","description":"Complete hoodie","price_kopecks":250000,"category":"hoodie","materials":["cotton"]}`, brand.ID), 201))
	path := fmt.Sprintf("/products/%d", product.ID)
	owner.call("GET", path, "", 404)
	if got := pvmDecode[[]managedColorResponse](t, editor.call("GET", path+"/colors", "", 200)); len(got) != 0 {
		t.Fatal("new draft has colors")
	}
	black := pvmDecode[managedColorResponse](t, editor.call("POST", path+"/colors", `{"color":" Black "}`, 201))
	if black.Color != "Black" || black.ID <= 0 || black.ProductID != product.ID || black.Images == nil {
		t.Fatal("color DTO")
	}
	cp := fmt.Sprintf("%s/colors/%d", path, black.ID)
	editor.call("POST", path+"/colors", `{"color":" bLaCk "}`, 409)
	editor.call("PATCH", cp, `{"color":" "}`, 400)
	editor.call("POST", cp+"/images", `{"url":"http://example.com/bad.jpg"}`, 400)
	front := pvmDecode[managedImageResponse](t, editor.call("POST", cp+"/images", `{"url":"https://example.com/front.jpg","position":0}`, 201))
	back := pvmDecode[managedImageResponse](t, owner.call("POST", cp+"/images", `{"url":"https://example.com/back.jpg","position":0}`, 201))
	if front.ID <= 0 || back.ID == front.ID {
		t.Fatal("image identity")
	}
	images := pvmDecode[[]managedImageResponse](t, editor.call("GET", cp+"/images", "", 200))
	if len(images) != 2 || images[0].ID != back.ID || images[1].ID != front.ID || images[1].Position != 1 {
		t.Fatal("insertion order")
	}
	editor.call("POST", cp+"/images", `{"url":"https://example.com/front.jpg"}`, 409)
	editor.call("POST", cp+"/images", `{"url":"https://example.com/new.jpg","position":9}`, 400)
	order := fmt.Sprintf(`{"image_ids":[%d,%d]}`, front.ID, back.ID)
	images = pvmDecode[[]managedImageResponse](t, editor.call("POST", cp+"/images/reorder", order, 200))
	if images[0].ID != front.ID || images[0].Position != 0 || images[1].ID != back.ID {
		t.Fatal("reorder identity")
	}
	editor.call("POST", cp+"/images/reorder", fmt.Sprintf(`{"image_ids":[%d,%d]}`, front.ID, front.ID), 400)
	editor.call("POST", cp+"/images/reorder", fmt.Sprintf(`{"image_ids":[%d]}`, front.ID), 400)
	editor.call("POST", cp+"/images/reorder", fmt.Sprintf(`{"image_ids":[%d,9223372036854775807]}`, front.ID), 404)
	fpath := fmt.Sprintf("%s/images/%d", cp, front.ID)
	editor.call("PATCH", fpath, `{"url":"https://example.com/back.jpg","position":1}`, 409)
	images = pvmDecode[[]managedImageResponse](t, owner.call("GET", cp+"/images", "", 200))
	if images[0].ID != front.ID || images[0].URL != front.URL {
		t.Fatal("failed patch changed order or URL")
	}
	changed := pvmDecode[managedImageResponse](t, editor.call("PATCH", fpath, `{"url":"https://example.com/front-v2.jpg","position":1}`, 200))
	if changed.ID != front.ID || changed.Position != 1 {
		t.Fatal("move changed identity")
	}
	editor.call("PATCH", fpath, `{}`, 400)
	v := pvmDecode[managedVariantResponse](t, editor.call("POST", cp+"/variants", `{"size":" M ","stock":12,"sku":"HOODIE-BLACK-M"}`, 201))
	v2 := pvmDecode[managedVariantResponse](t, editor.call("POST", cp+"/variants", `{"size":"One Size","stock":0}`, 201))
	if v.Size != "M" || v.Stock != 12 || v2.Stock != 0 || v2.SKU != "" {
		t.Fatal("variant DTO")
	}
	vpath := fmt.Sprintf("%s/variants/%d", cp, v.ID)
	editor.call("POST", cp+"/variants", `{"size":" m ","stock":0}`, 409)
	editor.call("POST", cp+"/variants", `{"size":"L","stock":-1}`, 400)
	editor.call("PATCH", vpath, `{"size":"one size","stock":1}`, 409)
	editor.call("PATCH", vpath, `{"stock":-1}`, 400)
	editor.call("PATCH", vpath, `{}`, 400)
	v = pvmDecode[managedVariantResponse](t, editor.call("PATCH", vpath, `{"size":"44 EU","sku":"same SKU allowed"}`, 200))
	if v.Size != "44 EU" || v.Stock != 12 || v.SKU != "same SKU allowed" {
		t.Fatal("partial variant patch")
	}
	editor.call("PATCH", vpath, `{"size":"M"}`, 200)
	// Another real draft of the same brand catches cross-product child IDs.
	other := pvmDecode[productResponse](t, owner.call("POST", "/products", fmt.Sprintf(`{"brand_id":%d,"name":"Other product"}`, brand.ID), 201))
	otherPath := fmt.Sprintf("/products/%d", other.ID)
	otherColor := pvmDecode[managedColorResponse](t, owner.call("POST", otherPath+"/colors", `{"color":"White"}`, 201))
	otherCP := fmt.Sprintf("%s/colors/%d", otherPath, otherColor.ID)
	otherV := pvmDecode[managedVariantResponse](t, owner.call("POST", otherCP+"/variants", `{"size":"M","stock":1}`, 201))
	otherImage := pvmDecode[managedImageResponse](t, owner.call("POST", otherCP+"/images", `{"url":"https://example.com/other.jpg"}`, 201))
	owner.call("PATCH", fmt.Sprintf("%s/colors/%d", path, otherColor.ID), `{"color":"Stolen"}`, 404)
	owner.call("PATCH", fmt.Sprintf("%s/variants/%d", cp, otherV.ID), `{"stock":10}`, 404)
	owner.call("DELETE", fmt.Sprintf("%s/images/%d", cp, otherImage.ID), "", 404)
	// A complete second brand still grants no access to this brand's product.
	outsider.call("POST", "/brands", `{"name":"Other brand","slug":"outsider-brand"}`, 201)
	for _, client := range []pvmHTTPClient{outsider, invited} {
		for _, r := range []struct{ method, path, body string }{
			{"GET", path + "/colors", ""}, {"POST", path + "/colors", `{"color":"Denied"}`}, {"PATCH", cp, `{"color":"Denied"}`}, {"DELETE", cp, ""},
			{"GET", cp + "/variants", ""}, {"POST", cp + "/variants", `{"size":"XL","stock":1}`}, {"PATCH", vpath, `{"stock":9}`}, {"DELETE", vpath, ""},
			{"GET", cp + "/images", ""}, {"POST", cp + "/images", `{"url":"https://example.com/denied.jpg"}`}, {"PATCH", fpath, `{"position":0}`}, {"DELETE", fpath, ""},
			{"POST", cp + "/images/reorder", order},
		} {
			client.call(r.method, r.path, r.body, 404)
		}
	}
	invited.call("POST", bpath+"/invitations/decline", `{}`, 200)
	invited.call("GET", path+"/colors", "", 404)
	editor.call("POST", path+"/publish", `{}`, 409) // brand is still draft
	owner.call("PATCH", bpath, `{"description":"PVM brand description","logo_url":"https://example.com/logo.jpg"}`, 200)
	owner.call("POST", bpath+"/publish", `{}`, 200)
	editor.call("POST", path+"/publish", `{}`, 200)
	guest := pvmHTTPClient{t: t, h: h}
	detail := pvmDecode[productDetailResponse](t, guest.call("GET", path, "", 200))
	if len(detail.Colors) != 1 || len(detail.Colors[0].Images) != 2 || detail.Colors[0].Images[0] != back.URL || len(detail.Colors[0].Variants) != 2 {
		t.Fatal("public detail does not reflect PVM")
	}
	before := detail.UpdatedAt
	editor.call("POST", path+"/colors", `{"color":"Incomplete published color"}`, 409)
	editor.call("DELETE", cp, "", 409)
	colors := pvmDecode[[]managedColorResponse](t, owner.call("GET", path+"/colors", "", 200))
	if len(colors) != 1 {
		t.Fatal("failed aggregate edit committed")
	}
	detail = pvmDecode[productDetailResponse](t, guest.call("GET", path, "", 200))
	if detail.UpdatedAt != before || len(detail.Colors[0].Variants) != 2 {
		t.Fatal("rollback lost children or touched product")
	}
	editor.call("PATCH", vpath, `{"stock":0}`, 200)
	browse := fmt.Sprintf("/products?brand_id=%d&size=M&color=black", brand.ID)
	if cards := pvmDecode[[]productCardResponse](t, guest.call("GET", browse, "", 200)); len(cards) != 0 {
		t.Fatal("sold-out variant matched")
	}
	cards := pvmDecode[[]productCardResponse](t, guest.call("GET", fmt.Sprintf("/products?brand_id=%d", brand.ID), "", 200))
	if len(cards) != 1 || cards[0].IsAvailable || len(cards[0].AvailableSizes) != 0 {
		t.Fatal("stock/card mismatch")
	}
	editor.call("PATCH", vpath, `{"stock":5}`, 200)
	cards = pvmDecode[[]productCardResponse](t, guest.call("GET", browse, "", 200))
	if len(cards) != 1 || !cards[0].IsAvailable || cards[0].PreviewImageURL != back.URL {
		t.Fatal("restock/filter/preview mismatch")
	}
	editor.call("DELETE", fpath, "", 204)
	editor.call("DELETE", fmt.Sprintf("%s/images/%d", cp, back.ID), "", 409)
	editor.call("DELETE", fmt.Sprintf("%s/variants/%d", cp, v2.ID), "", 204)
	editor.call("DELETE", vpath, "", 409)
	guest.call("GET", path, "", 200)
	// Complete a new color while hidden; publish and remove it atomically.
	editor.call("POST", path+"/hide", `{}`, 200)
	white := pvmDecode[managedColorResponse](t, editor.call("POST", path+"/colors", `{"color":"White"}`, 201))
	wcp := fmt.Sprintf("%s/colors/%d", path, white.ID)
	editor.call("POST", wcp+"/images", `{"url":"https://example.com/white.jpg"}`, 201)
	editor.call("POST", wcp+"/variants", `{"size":"M","stock":1}`, 201)
	editor.call("PATCH", wcp, `{"color":" black "}`, 409)
	editor.call("PATCH", wcp, `{"color":"Ivory"}`, 200)
	editor.call("POST", path+"/publish", `{}`, 200)
	editor.call("DELETE", wcp, "", 204)
	owner.call("GET", wcp+"/images", "", 404)
	guest.call("GET", path, "", 200)
	// Capacity and deletion of incomplete data are allowed on a draft.
	for i := 1; i < 10; i++ {
		owner.call("POST", otherCP+"/images", fmt.Sprintf(`{"url":"https://example.com/extra-%d.jpg"}`, i), 201)
	}
	owner.call("POST", otherCP+"/images", `{"url":"https://example.com/too-many.jpg"}`, 409)
	owner.call("DELETE", otherCP, "", 204)
	owner.call("GET", otherCP+"/variants", "", 404)
	owner.call("DELETE", fmt.Sprintf("%s/members/%d", bpath, editorID), "", 204)
	editor.call("PATCH", vpath, `{"stock":99}`, 404) // old access JWT, current DB rights
	editor.call("GET", cp+"/images", "", 404)
	owner.call("GET", cp+"/variants", "", 200)
}
