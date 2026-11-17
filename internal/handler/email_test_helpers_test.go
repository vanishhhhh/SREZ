package handler

import (
	"github.com/jmoiron/sqlx"
	"srez/internal/security"
	"srez/internal/testsupport"
	"srez/internal/usecase"
	"sync"
	"testing"
)

var testInboxes sync.Map

func authWithTestInbox(t *testing.T, h *Handler, db *sqlx.DB, p *security.Passwords, tokens *security.Tokens) *usecase.Auth {
	a, _, inbox := testsupport.NewAuth(db, p, tokens)
	testInboxes.Store(h, inbox)
	t.Cleanup(func() { testInboxes.Delete(h) })
	return a
}
func verifyTestAccount(t *testing.T, h *Handler, email string) {
	t.Helper()
	v, ok := testInboxes.Load(h)
	if !ok {
		t.Fatal("test mailbox missing")
	}
	raw := v.(*testsupport.Inbox).Token(email)
	if raw == "" {
		t.Fatal("verification message missing")
	}
	pvmHTTPClient{t: t, h: h}.call("POST", "/auth/verify-email", `{"token":"`+raw+`"}`, 200)
}
