package handler

import (
	"context"
	"net/http/httptest"
	"srez/internal/security"
	"srez/internal/usecase"
	"strings"
	"testing"
)

type shoppingProbe struct {
	usecase.ShoppingRepository
	calls int
	uid   int64
}

func (r *shoppingProbe) WithinCart(_ context.Context, uid int64, _ func(usecase.CartTx) error) error {
	r.calls++
	r.uid = uid
	return usecase.ErrResourceHidden
}
func TestShoppingBearerAndInput(t *testing.T) {
	h, tokens := secureHandler(t)
	repo := &shoppingProbe{}
	h.SetShopping(usecase.NewShopping(repo))
	token, err := tokens.Issue(7, security.Random())
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range []struct{ method, path string }{
		{"GET", "/favorites"}, {"POST", "/favorites/1"}, {"DELETE", "/favorites/1"}, {"GET", "/cart"},
		{"POST", "/cart/items"}, {"PATCH", "/cart/items/1"}, {"DELETE", "/cart/items/1"},
	} {
		w := httptest.NewRecorder()
		h.router.ServeHTTP(w, httptest.NewRequest(r.method, r.path, strings.NewReader(`{}`)))
		if w.Code != 401 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(r, w.Code)
		}
	}
	for _, body := range []string{`{}`, `{"product_variant_id":1}`, `{"product_variant_id":1,"quantity":null}`, `{"product_variant_id":1,"quantity":0}`, `{"product_variant_id":1,"quantity":-1}`, `{"product_variant_id":1,"quantity":101}`, `{"product_variant_id":1,"quantity":1.5}`, `{"product_variant_id":1,"quantity":"1"}`, `{"product_variant_id":1,"quantity":9223372036854775808}`, `{"product_variant_id":1,"quantity":1,"user_id":2}`, `{"product_variant_id":1,"quantity":1,"price_kopecks":1}`, `{"product_variant_id":1,"quantity":1,"color":"Black"}`, `{"product_variant_id":0,"quantity":1}`, `{} {}`} {
		req := httptest.NewRequest("POST", "/cart/items", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		h.router.ServeHTTP(w, req)
		if w.Code != 400 {
			t.Fatal(body, w.Code, w.Body.String())
		}
	}
	if repo.calls != 0 {
		t.Fatal("invalid body reached repository")
	}
	req := httptest.NewRequest("PATCH", "/cart/items/42", strings.NewReader(`{"quantity":1}`))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.router.ServeHTTP(w, req)
	if w.Code != 404 || repo.uid != 7 || repo.calls != 1 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, repo.uid, repo.calls)
	}
}
