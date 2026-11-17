package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"srez/internal/config"
	"srez/internal/security"
	"srez/internal/storage"
	"srez/internal/testsupport"
	"srez/internal/usecase"
)

type failingVerificationMail struct{}

func (failingVerificationMail) SendPasswordReset(context.Context, string, string) error {
	return errors.New("fixture smtp offline")
}

func (failingVerificationMail) SendVerification(context.Context, string, string) error {
	return errors.New("fixture smtp offline")
}
func TestPostgresHTTPEmailVerification(t *testing.T) {
	old, db := newPVMHTTP(t)
	_ = old
	inbox := &testsupport.Inbox{}
	repo := storage.NewAuth(db)
	tokens, _ := security.NewTokens(security.Digest("verify http jwt"), security.Digest("verify http csrf"), "srez", "api")
	verification := usecase.NewEmailVerification(repo, inbox, "http://127.0.0.1:8080", security.Digest("verify http ip"), usecase.DefaultVerificationPolicy())
	h := NewWithUsers(usecase.NewUsers(storage.NewUsers(db)))
	if e := h.EnableAuth(usecase.NewAuth(repo, security.NewPasswords(1), tokens, verification), tokens, config.Auth{Origin: "http://127.0.0.1:8080", LocalHTTP: true, Attempts: 100}); e != nil {
		t.Fatal(e)
	}
	call := pvmHTTPClient{t: t, h: h}
	reg := call.call("POST", "/auth/register", `{"email":"http_verify@example.com","username":"http_verify","display_name":"Verify","password":"long HTTP verification password"}`, 201)
	if bytes.Contains(reg.Body.Bytes(), []byte("access_token")) || len(reg.Result().Cookies()) != 0 || !bytes.Contains(reg.Body.Bytes(), []byte(`"email_delivery_status":"sent"`)) {
		t.Fatal("unexpected registration credentials/status")
	}
	call.call("POST", "/auth/login", `{"identifier":"http_verify","password":"wrong password"}`, 401)
	pending := call.call("POST", "/auth/login", `{"identifier":"http_verify","password":"long HTTP verification password"}`, 403)
	if len(pending.Result().Cookies()) != 0 {
		t.Fatal("pending refresh cookie")
	}
	raw := inbox.Token("http_verify@example.com")
	if raw == "" || bytes.Contains(reg.Body.Bytes(), []byte(raw)) {
		t.Fatal("token leak/missing mail")
	}
	for _, path := range []string{"/auth/verify-email", "/auth/resend-verification"} {
		request := httptest.NewRequest("POST", path, strings.NewReader(`{"token":"`+raw+`"}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", "http://evil.example")
		w := httptest.NewRecorder()
		h.router.ServeHTTP(w, request)
		if w.Code != 403 {
			t.Fatal("origin bypass")
		}
	}
	call.call("GET", "/auth/verify-email", "", 404)
	for _, body := range []string{`[]`, `{"token":"bad"}`, `{"token":"` + raw + `","user_id":999}`, `{"token":"` + raw + `","email_verified":true}`, `{"token":null}`} {
		call.call("POST", "/auth/verify-email", body, 400)
	}
	first := call.call("POST", "/auth/resend-verification", `{"identifier":"http_verify@example.com"}`, 202)
	unknown := call.call("POST", "/auth/resend-verification", `{"identifier":"not_found@example.com"}`, 202)
	if !bytes.Equal(first.Body.Bytes(), unknown.Body.Bytes()) {
		t.Fatal("account enumeration")
	}
	call.call("POST", "/auth/verify-email", `{"token":"`+raw+`"}`, 200)
	call.call("POST", "/auth/verify-email", `{"token":"`+raw+`"}`, 400)
	verified := call.call("POST", "/auth/resend-verification", `{"identifier":"http_verify@example.com"}`, 202)
	if !bytes.Equal(first.Body.Bytes(), verified.Body.Bytes()) {
		t.Fatal("verified account enumeration")
	}
	login := pvmDecode[usecase.AuthResult](t, call.call("POST", "/auth/login", `{"identifier":"http_verify","password":"long HTTP verification password"}`, 200))
	owner := pvmHTTPClient{t: t, h: h, token: login.AccessToken}
	profile := owner.call("GET", "/profile", "", 200)
	var dto map[string]any
	if e := json.Unmarshal(profile.Body.Bytes(), &dto); e != nil {
		t.Fatal(e)
	}
	if dto["email_verified_at"] == nil || dto["email_verification_required"] != true {
		t.Fatal("incorrect verification DTO")
	}
	if strings.Contains(profile.Body.String(), raw) || strings.Contains(profile.Body.String(), "token_hash") {
		t.Fatal("profile secret leak")
	}
}
func TestPostgresHTTPEmailSMTPFailureKeepsAccount(t *testing.T) {
	_, db := newPVMHTTP(t)
	repo := storage.NewAuth(db)
	tokens, _ := security.NewTokens(security.Digest("mail fail jwt"), security.Digest("mail fail csrf"), "srez", "api")
	v := usecase.NewEmailVerification(repo, failingVerificationMail{}, "http://127.0.0.1:8080", security.Digest("mail fail ip"), usecase.VerificationPolicy{IPMinute: 1, IPHour: 2, AccountHour: 5})
	h := NewWithUsers(usecase.NewUsers(storage.NewUsers(db)))
	if e := h.EnableAuth(usecase.NewAuth(repo, security.NewPasswords(1), tokens, v), tokens, config.Auth{Origin: "http://127.0.0.1:8080", LocalHTTP: true, Attempts: 100}); e != nil {
		t.Fatal(e)
	}
	c := pvmHTTPClient{t: t, h: h}
	r := c.call("POST", "/auth/register", `{"email":"offline@example.com","username":"offline","display_name":"Offline","password":"long offline password 123"}`, 201)
	if !strings.Contains(r.Body.String(), `"email_delivery_status":"unavailable"`) {
		t.Fatal("SMTP failure presented as sent")
	}
	u, e := repo.FindLogin(context.Background(), "offline")
	if e != nil || u == nil || u.CanAuthenticate() {
		t.Fatal("account lost or bypassed")
	}
	c.call("POST", "/auth/resend-verification", `{"identifier":"unknown@example.com"}`, 429)
	c.call("POST", "/auth/resend-verification", `{"identifier":"offline@example.com"}`, 429)
}
