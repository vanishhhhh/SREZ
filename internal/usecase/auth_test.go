package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"

	"srez/internal/entities"
	"srez/internal/security"
)

type authSpy struct{ calls int }

func (s *authSpy) Register(context.Context, entities.User) (*entities.User, error) {
	s.calls++
	return nil, ErrAccountExists
}
func (s *authSpy) FindLogin(context.Context, string) (*entities.User, error) {
	s.calls++
	return nil, nil
}
func (s *authSpy) Rehash(context.Context, int64, string, string) error { return nil }
func (s *authSpy) CreateSession(context.Context, entities.AuthSession, []byte) error {
	s.calls++
	return nil
}
func (s *authSpy) Session(context.Context, []byte) (*entities.AuthSession, error) {
	s.calls++
	return nil, nil
}
func (s *authSpy) Rotate(context.Context, []byte, []byte, string, bool) (*entities.AuthSession, error) {
	s.calls++
	return nil, ErrAuth
}
func TestAuthRejectsInvalidBeforeStorage(t *testing.T) {
	repo := &authSpy{}
	tokens, _ := security.NewTokens([]byte(strings.Repeat("a", 32)), []byte(strings.Repeat("b", 32)), "srez", "api")
	a := NewAuth(repo, security.NewPasswords(1), tokens)
	ctx := context.Background()
	for _, email := range []string{"invalid", "юзер@example.com", strings.Repeat("a", 255) + "@example.com"} {
		if _, err := a.Register(ctx, email, "valid_user", "Test", "a long enough password"); !errors.Is(err, entities.ErrInvalidValue) {
			t.Fatal(err)
		}
	}
	if _, err := a.Register(ctx, "valid@example.com", "valid_user", "Test", "short"); err == nil {
		t.Fatal("short password")
	}
	if _, err := a.Refresh(ctx, "bad", "csrf", false); !errors.Is(err, ErrAuth) {
		t.Fatal(err)
	}
	if _, err := a.CSRF(ctx, ""); !errors.Is(err, ErrAuth) {
		t.Fatal(err)
	}
	if repo.calls != 0 {
		t.Fatal("storage called for malformed request")
	}
	if _, err := a.Login(ctx, "missing", "a wrong long password"); !errors.Is(err, ErrAuth) {
		t.Fatal(err)
	}
	if repo.calls != 1 {
		t.Fatal("unexpected storage calls")
	}
}
