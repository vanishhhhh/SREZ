package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"srez/internal/security"
	"srez/internal/usecase"
)

func TestPVMRoutesRequireBearer(t *testing.T) {
	h, _ := secureHandler(t)
	for _, route := range []struct{ method, path string }{
		{"POST", "/products/1/colors"}, {"GET", "/products/1/colors"}, {"PATCH", "/products/1/colors/2"}, {"DELETE", "/products/1/colors/2"},
		{"POST", "/products/1/colors/2/variants"}, {"GET", "/products/1/colors/2/variants"}, {"PATCH", "/products/1/colors/2/variants/3"}, {"DELETE", "/products/1/colors/2/variants/3"},
		{"POST", "/products/1/colors/2/images"}, {"GET", "/products/1/colors/2/images"}, {"PATCH", "/products/1/colors/2/images/3"}, {"DELETE", "/products/1/colors/2/images/3"},
		{"POST", "/products/1/colors/2/images/reorder"},
	} {
		for _, token := range []string{"", "Bearer invalid"} {
			req := httptest.NewRequest(route.method, route.path, strings.NewReader(`{}`))
			req.Header.Set("Authorization", token)
			w := httptest.NewRecorder()
			h.router.ServeHTTP(w, req)
			if w.Code != 401 {
				t.Fatalf("%s %s: %d", route.method, route.path, w.Code)
			}
		}
	}
}
func TestPVMStrictInputAndIDValidation(t *testing.T) {
	h, tokens := secureHandler(t)
	repo := &rejectManagement{}
	h.SetManagement(usecase.NewManagement(repo))
	token, err := tokens.Issue(1, security.Random())
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		method, path, body string
		want               int
	}{
		{"POST", "/products/0/colors", `{"color":"Black"}`, 400},
		{"GET", "/products/9223372036854775808/colors", "", 400},
		{"GET", "/products/1/colors/-1/variants", "", 400},
		{"DELETE", "/products/1/colors/2/images/not-an-id", "", 400},
		{"PATCH", "/products/1/colors/2/variants/0", `{"stock":1}`, 400},
		{"POST", "/products/1/colors", `{"color":"Black","product_id":2}`, 400},
		{"PATCH", "/products/1/colors/2", `{"color":"Black","status":"published"}`, 400},
		{"POST", "/products/1/colors", `{"color":null}`, 400},
		{"POST", "/products/1/colors", `[]`, 400},
		{"POST", "/products/1/colors", `{"color":"Black"} {}`, 400},
		{"POST", "/products/1/colors/2/variants", `{"size":"M","stock":"12"}`, 400},
		{"POST", "/products/1/colors/2/variants", `{"size":"M","stock":9223372036854775808}`, 400},
		{"POST", "/products/1/colors/2/variants", `{"size":"M"}`, 400},
		{"PATCH", "/products/1/colors/2/variants/3", `{"product_color_id":3}`, 400},
		{"POST", "/products/1/colors/2/images", `{"url":"https://example.com/a.jpg","id":1}`, 400},
		{"PATCH", "/products/1/colors/2/images/3", `{"position":1.5}`, 400},
		{"POST", "/products/1/colors/2/images/reorder", `{"image_ids":[9223372036854775808]}`, 400},
		{"POST", "/products/1/colors/2/images/reorder", `{"image_ids":null}`, 400},
		{"POST", "/products/1/colors", `{"color":"` + strings.Repeat("a", 65536) + `"}`, 413},
	} {
		t.Run(tt.method+tt.path+tt.body[:min(len(tt.body), 60)], func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			h.router.ServeHTTP(w, req)
			if w.Code != tt.want {
				t.Fatalf("status %d, want %d", w.Code, tt.want)
			}
		})
	}
	if repo.calls != 0 {
		t.Fatal("invalid input reached repository", repo.calls)
	}
}
