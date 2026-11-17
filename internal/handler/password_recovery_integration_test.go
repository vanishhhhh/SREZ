package handler

import (
	"bytes"
	"context"
	"net/http/httptest"
	"srez/internal/config"
	"srez/internal/security"
	"srez/internal/storage"
	"srez/internal/testsupport"
	"srez/internal/usecase"
	"strings"
	"testing"
	"time"
)

func TestPostgresHTTPPasswordRecovery(t *testing.T) {
	_, db := newPVMHTTP(t)
	ctx := context.Background()
	tokens, _ := security.NewTokens(security.Digest("recovery HTTP jwt"), security.Digest("recovery HTTP csrf"), "srez", "api")
	auth, v, inbox := testsupport.NewAuth(db, security.NewPasswords(2), tokens)
	u, e := auth.Register(ctx, "http-reset@example.test", "http_reset", "Reset", "old password for HTTP reset")
	if e != nil {
		t.Fatal(e)
	}
	if e = inbox.Verify(ctx, v, u.Email()); e != nil {
		t.Fatal(e)
	}
	h := NewWithUsers(usecase.NewUsers(storage.NewUsers(db)))
	if e = h.EnableAuth(auth, tokens, config.Auth{Origin: "http://127.0.0.1:8080", LocalHTTP: true, Attempts: 100}); e != nil {
		t.Fatal(e)
	}
	c := pvmHTTPClient{t: t, h: h}
	login := pvmDecode[usecase.AuthResult](t, c.call("POST", "/auth/login", `{"identifier":"http_reset","password":"old password for HTTP reset"}`, 200))
	owner := pvmHTTPClient{t: t, h: h, token: login.AccessToken}
	owner.call("GET", "/profile", "", 200)
	first := c.call("POST", "/auth/forgot-password", `{"email":"http-reset@example.test"}`, 202)
	unknown := c.call("POST", "/auth/forgot-password", `{"email":"absent@example.test"}`, 202)
	if !bytes.Equal(first.Body.Bytes(), unknown.Body.Bytes()) || len(first.Result().Cookies()) != 0 {
		t.Fatal("forgot account disclosure/session")
	}
	for _, body := range []string{`[]`, `{"email":"bad"}`, `{"email":"http-reset@example.test","user_id":1}`} {
		c.call("POST", "/auth/forgot-password", body, 400)
	}
	for _, path := range []string{"/auth/forgot-password", "/auth/reset-password"} {
		r := httptest.NewRequest("POST", path, strings.NewReader(`{}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", "https://evil.example")
		w := httptest.NewRecorder()
		h.router.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal("recovery origin bypass")
		}
	}
	var raw string
	for i := 0; i < 100; i++ {
		raw = inbox.ResetToken(u.Email())
		if raw != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if raw == "" || bytes.Contains(first.Body.Bytes(), []byte(raw)) {
		t.Fatal("reset token missing/leaked")
	}
	for _, body := range []string{`{"token":"bad","password":"new valid password for HTTP"}`, `{"token":"` + raw + `","password":"short"}`, `{"token":"` + raw + `","password":"new valid password for HTTP","user_id":1}`, `{"token":"` + raw + `","password":"new valid password for HTTP","email":"other@example.test"}`, `{"token":"` + raw + `","password_hash":"x"}`} {
		c.call("POST", "/auth/reset-password", body, 400)
	}
	body := `{"token":"` + raw + `","password":"new valid password for HTTP"}`
	reset := c.call("POST", "/auth/reset-password", body, 200)
	if len(reset.Result().Cookies()) != 0 || bytes.Contains(reset.Body.Bytes(), []byte("access_token")) {
		t.Fatal("reset created authentication")
	}
	owner.call("GET", "/profile", "", 401)
	c.call("POST", "/auth/reset-password", body, 400)
	c.call("POST", "/auth/login", `{"identifier":"http_reset","password":"old password for HTTP reset"}`, 401)
	c.call("POST", "/auth/login", `{"identifier":"http_reset","password":"new valid password for HTTP"}`, 200)
}
