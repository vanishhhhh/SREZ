package storage_test

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"srez/internal/entities"
	"srez/internal/security"
	"srez/internal/storage"
	"srez/internal/testsupport"
	"srez/internal/usecase"
)

func TestPostgresEmailVerificationAtomicAndIssuanceGate(t *testing.T) {
	db := testDB(t)
	migrate(t, db)
	ctx := context.Background()
	tokens, _ := security.NewTokens(security.Digest("email jwt test"), security.Digest("email csrf test"), "srez", "api")
	auth, v, inbox := testsupport.NewAuth(db, security.NewPasswords(2), tokens)
	user, e := auth.Register(ctx, "verify@example.com", "verify_user", "Verify", "a long verification password")
	if e != nil {
		t.Fatal(e)
	}
	if !user.EmailVerificationRequired() || user.EmailVerifiedAt() != nil || user.CanAuthenticate() {
		t.Fatal("new account gate missing")
	}
	if _, e = auth.Login(ctx, "verify_user", "wrong password"); !errors.Is(e, usecase.ErrAuth) {
		t.Fatal("enumeration without password")
	}
	if _, e = auth.Login(ctx, "verify_user", "a long verification password"); !errors.Is(e, usecase.ErrEmailUnverified) {
		t.Fatal("pending login allowed", e)
	}
	if count(t, db, "auth_sessions") != 0 || count(t, db, "auth_refresh_tokens") != 0 {
		t.Fatal("pending session created")
	}
	raw := inbox.Token(user.Email())
	if !security.ValidRandom(raw) || len(raw) != 43 {
		t.Fatal("invalid verification entropy encoding")
	}
	var hash []byte
	var seconds float64
	if e = db.QueryRow(`SELECT token_hash,extract(epoch FROM expires_at-created_at) FROM email_verification_tokens WHERE user_id=$1`, user.ID()).Scan(&hash, &seconds); e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(hash, security.Digest(raw)) || bytes.Equal(hash, []byte(raw)) || seconds != 1800 {
		t.Fatal("hash or lifetime wrong")
	}
	var wg sync.WaitGroup
	results := make(chan error, 12)
	for range 12 {
		wg.Add(1)
		go func() { defer wg.Done(); results <- v.Verify(ctx, raw) }()
	}
	wg.Wait()
	close(results)
	success := 0
	for e := range results {
		if e == nil {
			success++
		} else if !errors.Is(e, usecase.ErrVerificationToken) {
			t.Fatal(e)
		}
	}
	if success != 1 {
		t.Fatal("token consumed more than once", success)
	}
	if e = v.Verify(ctx, raw); !errors.Is(e, usecase.ErrVerificationToken) {
		t.Fatal("replay allowed")
	}
	if _, e = auth.Login(ctx, "verify_user", "a long verification password"); e != nil {
		t.Fatal(e)
	}
	if count(t, db, "auth_sessions") != 1 {
		t.Fatal("verification created session")
	}
	login, e := auth.Login(ctx, "verify_user", "a long verification password")
	if e != nil {
		t.Fatal(e)
	}
	// Prove refresh and direct session creation also check eligibility, even if
	// an operator later marks a user pending while an old session exists.
	exec(t, db, `UPDATE users SET email_verified_at=NULL WHERE id=$1`, user.ID())
	if _, e = auth.Refresh(ctx, login.Refresh, login.CSRFToken, false); !errors.Is(e, usecase.ErrAuth) {
		t.Fatal("pending refresh issued JWT", e)
	}
	now := time.Now().UTC()
	e = storage.NewAuth(db).CreateSession(ctx, entities.AuthSession{ID: security.Random(), UserID: user.ID(), CreatedAt: now, ExpiresAt: now.Add(time.Hour), LastUsedAt: now}, security.Digest(security.Random()), user.PasswordHash())
	if !errors.Is(e, usecase.ErrEmailUnverified) {
		t.Fatal("direct pending session allowed", e)
	}
}
func TestPostgresEmailResendRotationExpiryAndPersistentLimits(t *testing.T) {
	db := testDB(t)
	migrate(t, db)
	ctx := context.Background()
	repo := storage.NewAuth(db)
	tokens, _ := security.NewTokens(security.Digest("rotate jwt"), security.Digest("rotate csrf"), "srez", "api")
	auth, v, inbox := testsupport.NewAuth(db, security.NewPasswords(1), tokens)
	u, e := auth.Register(ctx, "rotate@example.com", "rotate_user", "Rotate", "long rotate password 123")
	if e != nil {
		t.Fatal(e)
	}
	old := inbox.Token(u.Email())
	policy := usecase.VerificationPolicy{IPMinute: 100, IPHour: 100, AccountHour: 5}
	ip := security.Digest("isolated IP")
	if u, e := repo.RotateVerification(ctx, u.Email(), security.Digest(security.Random()), ip, policy); e != nil || u != nil {
		t.Fatal("60s cooldown missing", e)
	}
	exec(t, db, `UPDATE email_verification_tokens SET created_at=created_at-interval '61 seconds',expires_at=expires_at-interval '61 seconds'`)
	next := security.Random()
	if got, e := repo.RotateVerification(ctx, u.Email(), security.Digest(next), ip, policy); e != nil || got == nil {
		t.Fatal("rotation failed", e)
	}
	if e = v.Verify(ctx, old); !errors.Is(e, usecase.ErrVerificationToken) {
		t.Fatal("old link accepted")
	}
	exec(t, db, `UPDATE email_verification_tokens SET created_at=created_at-interval '31 minutes',expires_at=expires_at-interval '31 minutes' WHERE token_hash=$1`, security.Digest(next))
	if e = v.Verify(ctx, next); !errors.Is(e, usecase.ErrVerificationToken) {
		t.Fatal("expired link accepted")
	}
	// Expired and revoked tokens still count toward the account's hourly cap.
	for n := 2; n < 5; n++ {
		got, e := repo.RotateVerification(ctx, u.Email(), security.Digest(security.Random()), ip, policy)
		if e != nil || got == nil {
			t.Fatal("within account cap", e)
		}
		exec(t, db, `UPDATE email_verification_tokens SET created_at=created_at-interval '61 seconds',expires_at=expires_at-interval '61 seconds' WHERE user_id=$1 AND used_at IS NULL AND revoked_at IS NULL`, u.ID())
	}
	if got, e := repo.RotateVerification(ctx, u.Email(), security.Digest(security.Random()), ip, policy); e != nil || got != nil {
		t.Fatal("hourly account cap missing", e)
	}
	limitedIP := security.Digest("restart quota")
	strict := usecase.VerificationPolicy{IPMinute: 1, IPHour: 2, AccountHour: 5}
	if _, e = repo.RotateVerification(ctx, "unknown@example.com", security.Digest(security.Random()), limitedIP, strict); e != nil {
		t.Fatal(e)
	}
	// New repository, same persisted quota: unknown/verified emails cannot reset it.
	if _, e = storage.NewAuth(db).RotateVerification(ctx, "different@example.com", security.Digest(security.Random()), limitedIP, strict); !errors.Is(e, usecase.ErrVerificationRate) {
		t.Fatal("quota reset on restart", e)
	}
}
func TestPostgresEmailVerificationRollbackAndLegacy(t *testing.T) {
	db := testDB(t)
	migrate(t, db)
	ctx := context.Background()
	repo := storage.NewAuth(db)
	tokens, _ := security.NewTokens(security.Digest("rollback jwt"), security.Digest("rollback csrf"), "srez", "api")
	auth, v, inbox := testsupport.NewAuth(db, security.NewPasswords(1), tokens)
	user, e := auth.Register(ctx, "rollback@example.com", "rollback_user", "Rollback", "rollback password 123")
	if e != nil {
		t.Fatal(e)
	}
	raw := inbox.Token(user.Email())
	exec(t, db, `CREATE FUNCTION email_test_abort() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'isolated rollback fixture'; END; $$; CREATE TRIGGER email_test_abort BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION email_test_abort()`)
	if e = v.Verify(ctx, raw); e == nil {
		t.Fatal("expected transaction rollback")
	}
	var used *time.Time
	if e = db.Get(&used, `SELECT used_at FROM email_verification_tokens WHERE user_id=$1`, user.ID()); e != nil || used != nil {
		t.Fatal("token consumed on rollback", e)
	}
	exec(t, db, `DROP TRIGGER email_test_abort ON users`)
	if e = v.Verify(ctx, raw); e != nil {
		t.Fatal(e)
	}
	p := security.NewPasswords(1)
	hash, e := p.Hash(ctx, "old account password 123")
	if e != nil {
		t.Fatal(e)
	}
	legacy, _ := entities.NewUser("old@example.com", "old_user", "Old", hash, time.Now().UTC())
	saved, e := repo.Register(ctx, *legacy)
	if e != nil {
		t.Fatal(e)
	}
	if saved.EmailVerifiedAt() != nil || saved.EmailVerificationRequired() || !saved.CanAuthenticate() {
		t.Fatal("legacy account mislabelled")
	}
	if _, e = auth.Login(ctx, "old_user", "old account password 123"); e != nil {
		t.Fatal("old user blocked", e)
	}
	// Token collision rolls back the account INSERT and IP quota atomically.
	another, _ := entities.NewUser("atomic@example.com", "atomic_user", "Atomic", hash, time.Now().UTC())
	before := count(t, db, "users")
	if _, e = repo.RegisterUnverified(ctx, *another, security.Digest(raw), security.Digest("rollback ip"), usecase.DefaultVerificationPolicy()); e == nil {
		t.Fatal("expected unique token failure")
	}
	if count(t, db, "users") != before {
		t.Fatal("orphan account after rollback")
	}
}
