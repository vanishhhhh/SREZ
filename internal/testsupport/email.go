// Package testsupport supplies an in-memory mailbox, never an auth bypass.
// Test accounts still perform the normal one-time verification operation.
package testsupport

import (
	"context"
	"errors"
	"github.com/jmoiron/sqlx"
	"net/url"
	"srez/internal/security"
	"srez/internal/storage"
	"srez/internal/usecase"
	"sync"
)

type Inbox struct {
	mu         sync.Mutex
	links      map[string]string
	resetLinks map[string]string
}

func (i *Inbox) SendPasswordReset(_ context.Context, to, link string) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.resetLinks == nil {
		i.resetLinks = map[string]string{}
	}
	i.resetLinks[to] = link
	return nil
}
func (i *Inbox) ResetToken(email string) string {
	i.mu.Lock()
	defer i.mu.Unlock()
	u, e := url.Parse(i.resetLinks[email])
	if e != nil {
		return ""
	}
	q, e := url.ParseQuery(u.Fragment)
	if e != nil {
		return ""
	}
	return q.Get("token")
}

func (i *Inbox) SendVerification(_ context.Context, to, link string) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.links == nil {
		i.links = map[string]string{}
	}
	i.links[to] = link
	return nil
}
func (i *Inbox) Token(email string) string {
	i.mu.Lock()
	defer i.mu.Unlock()
	u, e := url.Parse(i.links[email])
	if e != nil {
		return ""
	}
	q, e := url.ParseQuery(u.Fragment)
	if e != nil {
		return ""
	}
	return q.Get("token")
}
func (i *Inbox) Verify(ctx context.Context, v *usecase.EmailVerification, email string) error {
	raw := i.Token(email)
	if raw == "" {
		return errors.New("test mailbox has no message")
	}
	return v.Verify(ctx, raw)
}
func NewAuth(db *sqlx.DB, p *security.Passwords, t *security.Tokens) (*usecase.Auth, *usecase.EmailVerification, *Inbox) {
	repo := storage.NewAuth(db)
	inbox := &Inbox{}
	v := usecase.NewEmailVerification(repo, inbox, "http://127.0.0.1:8080", security.Digest("isolated-test-email-ip-key"), usecase.VerificationPolicy{IPMinute: 1000, IPHour: 10000, AccountHour: 5})
	a := usecase.NewAuth(repo, p, t, v)
	a.SetRecovery(usecase.NewPasswordRecovery(repo, inbox, p, "http://127.0.0.1:8080", security.Digest("isolated-test-reset-ip-key"), usecase.VerificationPolicy{IPMinute: 1000, IPHour: 10000, AccountHour: 5}))
	return a, v, inbox
}
