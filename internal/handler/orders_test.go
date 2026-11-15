package handler

import (
	"context"
	"net/http/httptest"
	"srez/internal/security"
	"srez/internal/usecase"
	"strings"
	"testing"
)

type ordersProbe struct {
	usecase.OrderRepository
	uid   int64
	calls int
}

func (r *ordersProbe) WithinCheckout(_ context.Context, uid int64, _ func(usecase.CheckoutTx) error) error {
	r.uid = uid
	r.calls++
	return usecase.ErrEmptyCart
}
func TestOrdersBearerAndServerControlledFields(t *testing.T) {
	h, tokens := secureHandler(t)
	repo := &ordersProbe{}
	h.SetOrders(usecase.NewOrders(repo))
	for _, r := range []struct{ method, path string }{{"POST", "/orders"}, {"GET", "/orders"}, {"GET", "/orders/1"}} {
		w := httptest.NewRecorder()
		h.router.ServeHTTP(w, httptest.NewRequest(r.method, r.path, strings.NewReader(`{}`)))
		if w.Code != 401 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(r, w.Code)
		}
	}
	token, err := tokens.Issue(9, security.Random())
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{`{"user_id":2}`, `{"status":"paid"}`, `{"total_kopecks":1}`, `{"items":[]}`, `{"price":1}`, `null`, `{} {}`, ``} {
		req := httptest.NewRequest("POST", "/orders", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		h.router.ServeHTTP(w, req)
		if w.Code != 400 {
			t.Fatal(body, w.Code)
		}
	}
	if repo.calls != 0 {
		t.Fatal("client-controlled checkout reached storage")
	}
	req := httptest.NewRequest("POST", "/orders", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.router.ServeHTTP(w, req)
	if w.Code != 409 || repo.uid != 9 || repo.calls != 1 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, repo.uid, repo.calls)
	}
}
