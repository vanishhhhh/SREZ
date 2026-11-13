package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"srez/internal/entities"
	"strings"
	"testing"
	"time"
)

const firstProductJSON = `{"id":1,"brand_id":1,"name":"Футболка База","description":"Хлопковая футболка свободного кроя.","price_kopecks":250000,"category":"tshirt","materials":["хлопок"],"status":"published","created_at":"2026-10-28T12:00:00Z","updated_at":"2026-10-28T12:00:00Z"}`
const secondProductJSON = `{"id":2,"brand_id":2,"name":"Худи Город","description":"Худи с капюшоном для прохладной погоды.","price_kopecks":650000,"category":"hoodie","materials":["хлопок","полиэстер"],"status":"published","created_at":"2026-10-28T12:00:00Z","updated_at":"2026-10-28T12:00:00Z"}`
const firstProductColorsJSON = `[{"id":11,"color":"Чёрный","images":["https://example.com/products/1/black.png","https://example.com/products/1/black-back.png"],"variants":[{"id":101,"size":"M","stock":10,"sku":"SREZ-1-BLK-M"},{"id":102,"size":"L","stock":0,"sku":"SREZ-1-BLK-L"}]},{"id":12,"color":"Белый","images":["https://example.com/products/1/white.png"],"variants":[{"id":103,"size":"One Size","stock":4,"sku":"SREZ-1-WHT-OS"}]}]`
const secondProductColorsJSON = `[{"id":21,"color":"Чёрный","images":["https://example.com/products/2/black.png","https://example.com/products/2/black-back.png"],"variants":[{"id":201,"size":"M","stock":10,"sku":"SREZ-2-BLK-M"},{"id":202,"size":"L","stock":0,"sku":"SREZ-2-BLK-L"}]},{"id":22,"color":"Белый","images":["https://example.com/products/2/white.png"],"variants":[{"id":203,"size":"One Size","stock":4,"sku":"SREZ-2-WHT-OS"}]}]`
const firstBrandJSON = `{"id":1,"name":"Срез Студия","slug":"srez-studio","description":"Базовая одежда на каждый день.","logo_url":"https://example.com/brands/srez-studio/logo.png","banner_url":"https://example.com/brands/srez-studio/banner.png","contacts":{"email":"studio@example.com","telegram_url":"https://t.me/srez_example","website_url":"https://example.com/studio"},"appearance":{"accent_color":"#D14532","background_color":"#FFFFFF","text_color":"#222222"},"status":"active","created_at":"2026-10-28T12:00:00Z","updated_at":"2026-10-28T12:00:00Z"}`
const secondBrandJSON = `{"id":2,"name":"Линия","slug":"linea","description":"Удобные вещи для города.","logo_url":"https://example.com/brands/linea/logo.png","banner_url":"","contacts":{"email":"","telegram_url":"","website_url":"https://example.com/linea"},"appearance":{"accent_color":"#2455AA","background_color":"#F5F5F5","text_color":"#111111"},"status":"active","created_at":"2026-10-28T12:00:00Z","updated_at":"2026-10-28T12:00:00Z"}`

func catalogRequest(t *testing.T, h *Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	h.router.ServeHTTP(response, request)
	if !strings.HasPrefix(response.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("%s %s: expected JSON, got %q", method, path, response.Header().Get("Content-Type"))
	}
	if !json.Valid(response.Body.Bytes()) {
		t.Fatalf("invalid response JSON: %s", response.Body.String())
	}
	return response
}

func assertCatalogJSON(t *testing.T, got []byte, want string) {
	t.Helper()
	var actual, expected any
	if err := json.Unmarshal(got, &actual); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(want), &expected); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("JSON = %s, want %s", got, want)
	}
}

func assertCatalogError(t *testing.T, response *httptest.ResponseRecorder, status int) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, status, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	message, ok := body["error"].(string)
	if len(body) != 1 || !ok || strings.TrimSpace(message) == "" {
		t.Fatalf("expected only a nonempty error message: %s", response.Body.String())
	}
}

func TestCatalogGETEndpoints(t *testing.T) {
	h := newTestCatalogHandler()
	for _, tt := range []struct{ path, want string }{
		{"/health", `{"status":"ok"}`},
		{"/products", "[" + firstProductJSON + "," + secondProductJSON + "]"},
		{"/products/1", strings.TrimSuffix(firstProductJSON, "}") + `,"colors":` + firstProductColorsJSON + `}`},
		{"/products/2", strings.TrimSuffix(secondProductJSON, "}") + `,"colors":` + secondProductColorsJSON + `}`},
		{"/brands", "[" + firstBrandJSON + "," + secondBrandJSON + "]"},
		{"/brands/srez-studio", firstBrandJSON},
		{"/brands/linea", secondBrandJSON},
	} {
		t.Run(tt.path, func(t *testing.T) {
			response := catalogRequest(t, h, http.MethodGet, tt.path, "")
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d; body = %s", response.Code, response.Body.String())
			}
			// Exact keys also guard against accidental serialization of internal state.
			assertCatalogJSON(t, response.Body.Bytes(), tt.want)
		})
	}
}

func TestCatalogGETErrors(t *testing.T) {
	h := newTestCatalogHandler()
	for _, tt := range []struct {
		path   string
		status int
	}{
		{"/products/abc", 400}, {"/products/0", 400}, {"/products/-1", 400}, {"/products/1.5", 400},
		{"/products/9223372036854775808", 400}, {"/products/9223372036854775807", 404},
		{"/products/3", 404}, {"/products/999", 404},
		{"/brands/ab", 400}, {"/brands/" + strings.Repeat("a", 61), 400},
		{"/brands/Srez-Studio", 400}, {"/brands/srez_studio", 400},
		{"/brands/%D0%B1%D1%80%D0%B5%D0%BD%D0%B4", 400}, {"/brands/%20brand%20", 400},
		{"/brands/aaa", 404}, {"/brands/" + strings.Repeat("a", 60), 404}, {"/brands/missing-brand", 404},
	} {
		t.Run(tt.path, func(t *testing.T) {
			assertCatalogError(t, catalogRequest(t, h, http.MethodGet, tt.path, ""), tt.status)
		})
	}
}

func TestCreateProduct(t *testing.T) {
	for _, tt := range []struct{ name, body, want string }{
		{"full draft with normalization", `{"brand_id":1,"name":" Футболка ","description":" Описание ","price_kopecks":125000,"category":"tshirt","materials":[" хлопок "]}`,
			`{"id":3,"brand_id":1,"name":"Футболка","description":"Описание","price_kopecks":125000,"category":"tshirt","materials":["хлопок"],"status":"draft"}`},
		{"minimal draft", `{"brand_id":1,"name":"Товар"}`,
			`{"id":3,"brand_id":1,"name":"Товар","description":"","price_kopecks":0,"category":"","materials":[],"status":"draft"}`},
		{"zero price", `{"brand_id":2,"name":"Товар","price_kopecks":0,"category":"other","materials":[]}`,
			`{"id":3,"brand_id":2,"name":"Товар","description":"","price_kopecks":0,"category":"other","materials":[],"status":"draft"}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestCatalogHandler()
			response := catalogRequest(t, h, http.MethodPost, "/products", tt.body)
			if response.Code != http.StatusCreated {
				t.Fatalf("status = %d; body = %s", response.Code, response.Body.String())
			}
			var body map[string]json.RawMessage
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			var createdAt, updatedAt time.Time
			if err := json.Unmarshal(body["created_at"], &createdAt); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(body["updated_at"], &updatedAt); err != nil {
				t.Fatal(err)
			}
			if createdAt.IsZero() || !createdAt.Equal(updatedAt) {
				t.Fatalf("invalid draft timestamps: %s", response.Body.String())
			}
			delete(body, "created_at")
			delete(body, "updated_at")
			data, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			assertCatalogJSON(t, data, tt.want)
		})
	}
}

func TestCreateProductErrors(t *testing.T) {
	h := newTestCatalogHandler()
	for _, tt := range []struct{ name, body string }{
		{"empty body", ""}, {"broken JSON", "{"}, {"null", "null"}, {"array", "[]"}, {"scalar", "true"},
		{"two objects", `{"brand_id":1,"name":"Товар"} {}`},
		{"trailing garbage", `{"brand_id":1,"name":"Товар"} garbage`},
		{"missing fields", `{}`}, {"missing brand", `{"name":"Товар"}`},
		{"zero brand", `{"brand_id":0,"name":"Товар"}`}, {"negative brand", `{"brand_id":-1,"name":"Товар"}`},
		{"string brand", `{"brand_id":"1","name":"Товар"}`}, {"overflow brand", `{"brand_id":9223372036854775808,"name":"Товар"}`},
		{"missing name", `{"brand_id":1}`}, {"blank name", `{"brand_id":1,"name":"  "}`},
		{"short name after trimming", `{"brand_id":1,"name":" я "}`},
		{"long unicode name", `{"brand_id":1,"name":"` + strings.Repeat("я", 121) + `"}`},
		{"long description", `{"brand_id":1,"name":"Товар","description":"` + strings.Repeat("я", 5001) + `"}`},
		{"negative price", `{"brand_id":1,"name":"Товар","price_kopecks":-1}`},
		{"fractional price", `{"brand_id":1,"name":"Товар","price_kopecks":1.5}`},
		{"string price", `{"brand_id":1,"name":"Товар","price_kopecks":"100"}`},
		{"overflow price", `{"brand_id":1,"name":"Товар","price_kopecks":9223372036854775808}`},
		{"unknown category", `{"brand_id":1,"name":"Товар","category":"unknown"}`},
		{"empty material", `{"brand_id":1,"name":"Товар","materials":[" "]}`},
		{"duplicate material", `{"brand_id":1,"name":"Товар","materials":["cotton"," cotton "]}`},
		{"too many materials", `{"brand_id":1,"name":"Товар","materials":["a","b","c","d","e","f","g","h","i","j","k"]}`},
		{"material type", `{"brand_id":1,"name":"Товар","materials":[1]}`},
		{"unknown field", `{"brand_id":1,"name":"Товар","extra":true}`},
		{"client ID", `{"brand_id":1,"name":"Товар","id":1}`},
		{"client publication", `{"brand_id":1,"name":"Товар","status":"published"}`},
		{"sensitive field", `{"brand_id":1,"name":"Товар","password_hash":"secret"}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assertCatalogError(t, catalogRequest(t, h, http.MethodPost, "/products", tt.body), http.StatusBadRequest)
		})
	}
}

func TestDraftsAreSavedButNotPublic(t *testing.T) {
	h := newTestCatalogHandler()
	paths := []string{"/products", "/products/1", "/products/2", "/brands", "/brands/srez-studio", "/brands/linea"}
	before := make(map[string]string, len(paths))
	for _, path := range paths {
		before[path] = catalogRequest(t, h, http.MethodGet, path, "").Body.String()
	}
	for id := int64(3); id <= 4; id++ {
		response := catalogRequest(t, h, http.MethodPost, "/products", `{"brand_id":1,"name":"Новый товар"}`)
		if response.Code != http.StatusCreated {
			t.Fatalf("status = %d", response.Code)
		}
		var draft struct {
			ID     int64  `json:"id"`
			Status string `json:"status"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &draft); err != nil {
			t.Fatal(err)
		}
		if draft.ID != id || draft.Status != "draft" {
			t.Fatalf("unexpected draft: %s", response.Body.String())
		}
		assertCatalogError(t, catalogRequest(t, h, http.MethodGet, "/products/3", ""), http.StatusNotFound)
	}
	assertCatalogError(t, catalogRequest(t, h, http.MethodPost, "/products", `{"brand_id":1,"name":" "}`), http.StatusBadRequest)
	for _, path := range paths {
		response := catalogRequest(t, h, http.MethodGet, path, "")
		if response.Code != http.StatusOK || response.Body.String() != before[path] {
			t.Fatalf("public catalog changed after POST: %s: %s", path, response.Body.String())
		}
	}
}

func TestProductDetailVariantsBelongToColors(t *testing.T) {
	entries, err := mockProductCatalog()
	if err != nil {
		t.Fatal(err)
	}
	h := newTestCatalogHandler()
	for _, entry := range entries {
		path := "/products/1"
		if entry.product.ID() == 2 {
			path = "/products/2"
		}
		response := catalogRequest(t, h, http.MethodGet, path, "")
		var detail productDetailResponse
		if err := json.Unmarshal(response.Body.Bytes(), &detail); err != nil {
			t.Fatal(err)
		}
		count := 0
		for _, color := range detail.Colors {
			for _, variant := range color.Variants {
				found := false
				for _, domain := range entry.publication.Variants {
					if domain.ID() == variant.ID {
						if domain.ProductColorID() != color.ID || domain.Size() != variant.Size || domain.Stock() != variant.Stock || domain.SKU() != variant.SKU {
							t.Fatalf("variant does not match its domain color: %#v, color %d", variant, color.ID)
						}
						found = true
					}
				}
				if !found {
					t.Fatalf("foreign variant: %#v", variant)
				}
				count++
			}
		}
		if count != len(entry.publication.Variants) {
			t.Fatalf("variants = %d, want %d", count, len(entry.publication.Variants))
		}
	}
}

func TestCreateProductBodyLimit(t *testing.T) {
	const limit = 64 * 1024
	valid := `{"brand_id":1,"name":"Товар"}`
	for _, tt := range []struct {
		name, body string
		status     int
	}{
		{"exact limit", valid + strings.Repeat(" ", limit-len(valid)), 201},
		{"one byte over", valid + strings.Repeat(" ", limit-len(valid)+1), 413},
		{"oversized JSON string", `{"brand_id":1,"name":"` + strings.Repeat("я", limit) + `"}`, 413},
		{"oversized malformed JSON", "!" + strings.Repeat(" ", limit), 413},
		{"malformed at limit", "!" + strings.Repeat(" ", limit-1), 400},
		{"second JSON below limit", valid + " {}", 400},
	} {
		for _, unknownLength := range []bool{false, true} {
			t.Run(tt.name+map[bool]string{false: "/known length", true: "/unknown length"}[unknownLength], func(t *testing.T) {
				request := httptest.NewRequest(http.MethodPost, "/products", strings.NewReader(tt.body))
				request.Header.Set("Content-Type", "application/json")
				if unknownLength {
					request.ContentLength = -1
					request.TransferEncoding = []string{"chunked"}
				}
				response := httptest.NewRecorder()
				newTestCatalogHandler().router.ServeHTTP(response, request)
				if !strings.HasPrefix(response.Header().Get("Content-Type"), "application/json") {
					t.Fatal("expected JSON response")
				}
				if tt.status >= 400 {
					assertCatalogError(t, response, tt.status)
				} else if response.Code != tt.status {
					t.Fatalf("status = %d, want %d: %s", response.Code, tt.status, response.Body.String())
				}
			})
		}
	}
}

func TestCatalogWriteRequiresJWT(t *testing.T) {
	t.Setenv("SREZ_DEV_CATALOG_WRITE", "true")
	h, _ := secureHandler(t)
	assertCatalogError(t, catalogRequest(t, h, "POST", "/products", `{"brand_id":1,"name":"Товар"}`), 401)
}

func TestCatalogBrandChecks(t *testing.T) {
	h := newTestCatalogHandler()
	assertCatalogError(t, catalogRequest(t, h, "POST", "/products", `{"brand_id":999,"name":"Товар"}`), 404)
	r := newCatalogRepository()
	state := r.brands[0].State()
	state.Status = "suspended"
	b, err := entities.RestoreBrand(state)
	if err != nil {
		t.Fatal(err)
	}
	r.brands[0] = *b
	h = fixtureCatalogHandler(r)
	assertCatalogError(t, catalogRequest(t, h, "POST", "/products", `{"brand_id":1,"name":"Товар"}`), 409)
	assertCatalogError(t, catalogRequest(t, h, "GET", "/brands/srez-studio", ""), 404)
	assertCatalogError(t, catalogRequest(t, h, "GET", "/products/1", ""), 404)
}

func TestCatalogStorageErrorsArePrivate(t *testing.T) {
	r := newCatalogRepository()
	r.err = errors.New("secret SQL connection details")
	h := fixtureCatalogHandler(r)
	for _, tt := range []struct{ method, path, body string }{
		{"GET", "/brands", ""}, {"GET", "/brands/srez-studio", ""}, {"GET", "/products", ""}, {"GET", "/products/1", ""}, {"POST", "/products", `{"brand_id":1,"name":"Товар"}`},
	} {
		response := catalogRequest(t, h, tt.method, tt.path, tt.body)
		assertCatalogError(t, response, 500)
		if strings.Contains(response.Body.String(), "secret") {
			t.Fatal("storage error leaked")
		}
	}
}
