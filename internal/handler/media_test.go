package handler

import (
	"net/http/httptest"
	"srez/internal/security"
	"srez/internal/usecase"
	"strings"
	"testing"
)

func TestMediaRequiresJWTAndRejectsClientKeys(t *testing.T) {
	h, tokens := secureHandler(t)
	h.SetMedia(usecase.NewMedia(nil, nil, "test"))
	for _, path := range []string{"/media/uploads", "/media/uploads/1/complete"} {
		for _, header := range []string{"", "Bearer invalid"} {
			r := httptest.NewRequest("POST", path, strings.NewReader(`{}`))
			r.Header.Set("Authorization", header)
			w := httptest.NewRecorder()
			h.router.ServeHTTP(w, r)
			if w.Code != 401 {
				t.Fatal(path, w.Code)
			}
		}
	}
	token, e := tokens.Issue(1, security.Random())
	if e != nil {
		t.Fatal(e)
	}
	for _, body := range []string{`{"product_id":1,"color_id":1,"content_type":"image/jpeg","size_bytes":1,"object_key":"ready/overwrite"}`, `{"user_id":2}`, `{"bucket":"other"}`} {
		r := httptest.NewRequest("POST", "/media/uploads", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		h.router.ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}

func TestMediaBackendBodyGuards(t *testing.T) {
	h, tokens := secureHandler(t)
	h.SetMedia(usecase.NewMedia(nil, nil, "test"))
	token, e := tokens.Issue(1, security.Random())
	if e != nil {
		t.Fatal(e)
	}
	for _, tt := range []struct {
		name    string
		length  int64
		chunked bool
		want    int
	}{
		{"early oversize", usecase.MaxMediaBytes + 1, false, 413},
		{"missing length", -1, false, 411},
		{"chunked", 10, true, 411},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("PUT", "/media/uploads/1/content", strings.NewReader("test"))
			r.ContentLength = tt.length
			if tt.chunked {
				r.TransferEncoding = []string{"chunked"}
			}
			r.Header.Set("Authorization", "Bearer "+token)
			r.Header.Set("Content-Type", "image/jpeg")
			w := httptest.NewRecorder()
			h.router.ServeHTTP(w, r)
			if w.Code != tt.want {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
	for _, target := range []struct{ method, path string }{{"PUT", "/media/uploads/1/content"}, {"DELETE", "/profile/avatar"}, {"DELETE", "/brands/1/media/brand_logo"}} {
		w := httptest.NewRecorder()
		h.router.ServeHTTP(w, httptest.NewRequest(target.method, target.path, nil))
		if w.Code != 401 {
			t.Fatal("JWT bypass", target, w.Code)
		}
	}
}
