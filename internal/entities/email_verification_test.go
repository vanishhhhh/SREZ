package entities

import (
	"testing"
	"time"
)

func TestEmailVerificationAccountRulesAndDetachedState(t *testing.T) {
	now := time.Now().UTC()
	s := UserState{ID: 1, Email: "email@example.com", Username: "email_user", DisplayName: "Email", PasswordHash: "!", CreatedAt: now, UpdatedAt: now}
	legacy, e := RestoreUser(s)
	if e != nil || !legacy.CanAuthenticate() || legacy.EmailVerifiedAt() != nil {
		t.Fatal("legacy mislabelled")
	}
	s.EmailVerificationRequired = true
	pending, e := RestoreUser(s)
	if e != nil || pending.CanAuthenticate() {
		t.Fatal("pending allowed")
	}
	s.EmailVerifiedAt = &now
	verified, e := RestoreUser(s)
	if e != nil || !verified.CanAuthenticate() {
		t.Fatal("verified blocked")
	}
	snapshot := verified.State()
	*snapshot.EmailVerifiedAt = now.Add(-time.Hour)
	if verified.EmailVerifiedAt().Before(now) {
		t.Fatal("mutable snapshot")
	}
	s.EmailVerifiedAt = &snapshot.CreatedAt
	s.UpdatedAt = now.Add(-time.Second)
	if _, e = RestoreUser(s); e == nil {
		t.Fatal("invalid verification timestamp")
	}
}
