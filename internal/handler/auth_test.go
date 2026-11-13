package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"srez/internal/config"
	"srez/internal/entities"
	"srez/internal/security"
	"srez/internal/usecase"
)

type noAuthRepo struct{}

func (noAuthRepo) Register(context.Context, entities.User) (*entities.User, error) {
	return nil, usecase.ErrAccountExists
}
func (noAuthRepo) FindLogin(context.Context, string) (*entities.User, error)         { return nil, nil }
func (noAuthRepo) Rehash(context.Context, int64, string, string) error               { return nil }
func (noAuthRepo) CreateSession(context.Context, entities.AuthSession, []byte) error { return nil }
func (noAuthRepo) Session(context.Context, []byte) (*entities.AuthSession, error)    { return nil, nil }
func (noAuthRepo) Rotate(context.Context, []byte, []byte, string, bool) (*entities.AuthSession, error) {
	return nil, usecase.ErrAuth
}
func secureHandler(t *testing.T) (*Handler, *security.Tokens) {
	t.Helper()
	tokens, e := security.NewTokens([]byte(strings.Repeat("a", 32)), []byte(strings.Repeat("b", 32)), "issuer", "audience")
	if e != nil {
		t.Fatal(e)
	}
	h := NewWithUsers(usecase.NewUsers(mockUserRepository{}))
	cfg := config.Auth{Origin: "http://127.0.0.1:8080", LocalHTTP: true, Attempts: 2}
	if e = h.EnableAuth(usecase.NewAuth(noAuthRepo{}, security.NewPasswords(1), tokens), tokens, cfg); e != nil {
		t.Fatal(e)
	}
	return h, tokens
}
func TestAuthHTTPProtection(t *testing.T) {
	h, tokens := secureHandler(t)
	for _, tt := range []struct {
		method, path string
		status       int
	}{{"GET", "/users", 404}, {"POST", "/users", 404}, {"PUT", "/users/2", 404}, {"DELETE", "/users/2", 404}, {"GET", "/profile", 401}, {"PATCH", "/profile", 401}} {
		r := httptest.NewRecorder()
		h.router.ServeHTTP(r, httptest.NewRequest(tt.method, tt.path, nil))
		if r.Code != tt.status {
			t.Fatal(tt, r.Code)
		}
	}
	for _, header := range []string{"Bearer", "Basic abc", "Bearer not-a-jwt", "Bearer a b"} {
		request := httptest.NewRequest("GET", "/profile", nil)
		request.Header.Set("Authorization", header)
		response := httptest.NewRecorder()
		h.router.ServeHTTP(response, request)
		if response.Code != 401 {
			t.Fatal(response.Code)
		}
	}
	request := httptest.NewRequest("GET", "/auth/csrf", nil)
	request.Header.Set("Origin", "http://127.0.0.1:8080")
	request.Header.Set("Sec-Fetch-Site", "cross-site")
	response := httptest.NewRecorder()
	h.router.ServeHTTP(response, request)
	if response.Code != 403 {
		t.Fatal(response.Code)
	}
	raw, _ := tokens.Issue(1, security.Random())
	r := httptest.NewRequest("GET", "/profile", nil)
	r.Header.Set("Authorization", "Bearer "+raw)
	w := httptest.NewRecorder()
	h.router.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "demo@example.com") {
		t.Fatal(w.Code, w.Body.String())
	}
	r = httptest.NewRequest("PATCH", "/profile", strings.NewReader(`{"user_id":2,"display_name":"Hacked"}`))
	r.Header.Set("Authorization", "Bearer "+raw)
	w = httptest.NewRecorder()
	h.router.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
	for _, origin := range []string{"", "http://evil.test"} {
		r = httptest.NewRequest("POST", "/auth/login", strings.NewReader(`{}`))
		r.Header.Set("Origin", origin)
		r.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		h.router.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal(w.Code)
		}
	}
	for i := 0; i < 3; i++ {
		r = httptest.NewRequest("POST", "/auth/login", strings.NewReader(`{"identifier":"missing","password":"wrong"}`))
		r.Header.Set("Origin", "http://127.0.0.1:8080")
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Forwarded-For", security.Random())
		w = httptest.NewRecorder()
		h.router.ServeHTTP(w, r)
		want := 401
		if i == 2 {
			want = 429
		}
		if w.Code != want {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
func TestCookieAttributes(t *testing.T) {
	for _, local := range []bool{true, false} {
		x := &authHTTP{cfg: config.Auth{LocalHTTP: local}}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		x.cookie(c, security.Random(), time.Now().Add(time.Hour))
		cookie := w.Result().Cookies()[0]
		if !cookie.HttpOnly || cookie.Secure == local || cookie.Path != "/" || cookie.Domain != "" || cookie.SameSite != http.SameSiteStrictMode {
			t.Fatal(cookie)
		}
		if !local && cookie.Name != "__Host-srez_refresh" {
			t.Fatal(cookie.Name)
		}
	}
}
