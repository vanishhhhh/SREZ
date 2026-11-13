package handler

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"srez/internal/entities"
	"srez/internal/security"
	"srez/internal/usecase"
)

type rejectManagement struct {
	usecase.ManagementRepository
	calls int
}

func (r *rejectManagement) CreateBrand(context.Context, int64, entities.Brand) (*entities.Brand, error) {
	r.calls++
	return nil, usecase.ErrResourceHidden
}
func (r *rejectManagement) WithinBrand(context.Context, int64, func(usecase.ManagementTx, *entities.Brand) error) error {
	r.calls++
	return usecase.ErrResourceHidden
}
func (r *rejectManagement) ProductBrand(context.Context, int64) (int64, error) {
	r.calls++
	return 0, usecase.ErrResourceHidden
}
func TestManagementRoutesRequireBearer(t *testing.T) {
	t.Setenv("SREZ_DEV_CATALOG_WRITE", "true")
	h, _ := secureHandler(t)
	for _, tt := range []struct{ method, path string }{{"POST", "/brands"}, {"GET", "/me/brands"}, {"GET", "/me/brand-invitations"}, {"GET", "/brands/1/manage"}, {"GET", "/brands/1/members"}, {"GET", "/brands/1/products/manage"}, {"PATCH", "/brands/1"}, {"POST", "/brands/1/publish"}, {"POST", "/brands/1/unpublish"}, {"POST", "/brands/1/invitations"}, {"POST", "/brands/1/invitations/accept"}, {"POST", "/brands/1/invitations/decline"}, {"DELETE", "/brands/1/members/2"}, {"POST", "/products"}, {"PATCH", "/products/1"}, {"POST", "/products/1/publish"}, {"POST", "/products/1/hide"}, {"POST", "/products/1/unpublish"}} {
		t.Run(tt.method+tt.path, func(t *testing.T) {
			r := httptest.NewRecorder()
			h.router.ServeHTTP(r, httptest.NewRequest(tt.method, tt.path, strings.NewReader(`{}`)))
			if r.Code != 401 {
				t.Fatal(r.Code)
			}
		})
	}
}
func TestManagementMassAssignment(t *testing.T) {
	h, tokens := secureHandler(t)
	repo := &rejectManagement{}
	h.SetManagement(usecase.NewManagement(repo))
	token, e := tokens.Issue(1, security.Random())
	if e != nil {
		t.Fatal(e)
	}
	for _, tt := range []struct{ method, path, body string }{{"POST", "/brands", `{"name":"Brand","slug":"brand","owner_user_id":2}`}, {"POST", "/brands", `{"name":"Brand","role":"owner"}`}, {"PATCH", "/brands/1", `{"status":"active"}`}, {"PATCH", "/brands/1", `{"contacts":{"email":"test@example.com","role":"owner"}}`}, {"POST", "/brands/1/invitations", `{"username":"editor","role":"owner"}`}, {"POST", "/products", `{"brand_id":1,"name":"Product","status":"published"}`}, {"PATCH", "/products/1", `{"brand_id":2}`}, {"PATCH", "/products/1", `{"owner":2}`}, {"POST", "/products/1/publish", `{"is_ready":true}`}} {
		t.Run(tt.path+tt.body, func(t *testing.T) {
			r := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			h.router.ServeHTTP(w, r)
			if w.Code != 400 {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
	if repo.calls != 0 {
		t.Fatal("mass assignment reached repository")
	}
}
