package storage_test

import (
	"bytes"
	"context"
	"errors"
	"srez/internal/entities"
	"srez/internal/security"
	"srez/internal/storage"
	"srez/internal/testsupport"
	"srez/internal/usecase"
	"sync"
	"testing"
	"time"
)

func TestPostgresPasswordRecoveryRevokesAllCredentials(t *testing.T) {
	db := testDB(t)
	migrate(t, db)
	ctx := context.Background()
	tokens, _ := security.NewTokens(security.Digest("recovery jwt"), security.Digest("recovery csrf"), "srez", "api")
	auth, v, inbox := testsupport.NewAuth(db, security.NewPasswords(2), tokens)
	const email = "recovery@example.test"
	const old = "old recovery password 123"
	const next = "new recovery password 456"
	u, e := auth.Register(ctx, email, "recovery", "Recovery", old)
	if e != nil {
		t.Fatal(e)
	}
	if e = auth.ForgotPassword(ctx, email, "127.0.0.1"); e != nil {
		t.Fatal(e)
	}
	if count(t, db, "password_reset_tokens") != 0 {
		t.Fatal("pending email eligible")
	}
	verificationToken := inbox.Token(email)
	if e = inbox.Verify(ctx, v, email); e != nil {
		t.Fatal(e)
	}
	one, e := auth.Login(ctx, email, old)
	if e != nil {
		t.Fatal(e)
	}
	two, e := auth.Login(ctx, email, old)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = auth.Authenticate(ctx, one.AccessToken); e != nil {
		t.Fatal(e)
	}
	if e = auth.ForgotPassword(ctx, email, "127.0.0.1"); e != nil {
		t.Fatal(e)
	}
	var raw string
	for i := 0; i < 100; i++ {
		raw = inbox.ResetToken(email)
		if raw != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !security.ValidRandom(raw) {
		t.Fatal("missing reset mail")
	}
	var hash []byte
	var seconds float64
	if e = db.QueryRow(`SELECT token_hash,extract(epoch FROM expires_at-created_at) FROM password_reset_tokens WHERE user_id=$1`, u.ID()).Scan(&hash, &seconds); e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(hash, security.Digest(raw)) || bytes.Equal(hash, []byte(raw)) || seconds != 1800 {
		t.Fatal("reset hash/expiry")
	}
	if e = v.Verify(ctx, raw); !errors.Is(e, usecase.ErrVerificationToken) {
		t.Fatal("reset verifies email", e)
	}
	if e = auth.ResetPassword(ctx, verificationToken, next); !errors.Is(e, usecase.ErrResetToken) {
		t.Fatal("verification resets password", e)
	}
	if e = auth.ResetPassword(ctx, raw, next); e != nil {
		t.Fatal(e)
	}
	if e = auth.ResetPassword(ctx, raw, next); !errors.Is(e, usecase.ErrResetToken) {
		t.Fatal("reset replay", e)
	}
	for _, s := range []*usecase.AuthResult{one, two} {
		if _, e = auth.Authenticate(ctx, s.AccessToken); !errors.Is(e, usecase.ErrAuth) {
			t.Fatal("old access accepted", e)
		}
		if _, e = auth.Refresh(ctx, s.Refresh, s.CSRFToken, false); !errors.Is(e, usecase.ErrAuth) {
			t.Fatal("old refresh accepted", e)
		}
	}
	if _, e = auth.Login(ctx, email, old); !errors.Is(e, usecase.ErrAuth) {
		t.Fatal("old password accepted", e)
	}
	if count(t, db, "auth_sessions") != 2 {
		t.Fatal("reset automatically created session")
	}
	now := time.Now().UTC()
	// A login which checked the old password before reset cannot create a new
	// session after reset. This closes the stale-password issuance race.
	e = storage.NewAuth(db).CreateSession(ctx, entities.AuthSession{ID: security.Random(), UserID: u.ID(), CreatedAt: now, LastUsedAt: now, ExpiresAt: now.Add(time.Hour)}, security.Digest(security.Random()), u.PasswordHash())
	if !errors.Is(e, usecase.ErrAuth) {
		t.Fatal("stale password login race", e)
	}
	fresh, e := auth.Login(ctx, email, next)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = auth.Authenticate(ctx, fresh.AccessToken); e != nil {
		t.Fatal(e)
	}
	if _, e = auth.Refresh(ctx, fresh.Refresh, fresh.CSRFToken, true); e != nil {
		t.Fatal(e)
	}
	if _, e = auth.Authenticate(ctx, fresh.AccessToken); !errors.Is(e, usecase.ErrAuth) {
		t.Fatal("logout access retained", e)
	}
}

func TestPostgresPasswordRecoveryAtomicityConcurrencyAndRefreshRace(t *testing.T) {
	db := testDB(t)
	migrate(t, db)
	ctx := context.Background()
	repo := storage.NewAuth(db)
	tokens, _ := security.NewTokens(security.Digest("race jwt"), security.Digest("race csrf"), "srez", "api")
	passwords := security.NewPasswords(2)
	auth, v, inbox := testsupport.NewAuth(db, passwords, tokens)
	u, e := auth.Register(ctx, "reset-race@example.test", "reset_race", "Race", "old password for race 123")
	if e != nil {
		t.Fatal(e)
	}
	if e = inbox.Verify(ctx, v, u.Email()); e != nil {
		t.Fatal(e)
	}
	login, e := auth.Login(ctx, u.Email(), "old password for race 123")
	if e != nil {
		t.Fatal(e)
	}
	raw := security.Random()
	p := usecase.DefaultVerificationPolicy()
	if _, e = repo.IssuePasswordReset(ctx, u.Email(), security.Digest(raw), security.Digest("race ip"), p); e != nil {
		t.Fatal(e)
	}
	newHash, e := passwords.Hash(ctx, "new atomic password 456")
	if e != nil {
		t.Fatal(e)
	}
	exec(t, db, `CREATE FUNCTION reject_password() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.password_hash<>OLD.password_hash THEN RAISE EXCEPTION 'fixture rollback'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_password BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION reject_password()`)
	if e = repo.ResetPassword(ctx, security.Digest(raw), newHash); e == nil {
		t.Fatal("rollback fixture ignored")
	}
	var used bool
	if e = db.Get(&used, `SELECT used_at IS NOT NULL FROM password_reset_tokens WHERE user_id=$1`, u.ID()); e != nil || used {
		t.Fatal("token consumed on rollback")
	}
	if _, e = auth.Authenticate(ctx, login.AccessToken); e != nil {
		t.Fatal("session lost on rollback", e)
	}
	exec(t, db, `DROP TRIGGER reject_password ON users;DROP FUNCTION reject_password()`)
	results := make(chan error, 12)
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() { defer wg.Done(); results <- repo.ResetPassword(ctx, security.Digest(raw), newHash) }()
	}
	// Concurrent refresh must not deadlock reset or leave usable credentials.
	refreshed := make(chan *usecase.AuthResult, 1)
	refreshErr := make(chan error, 1)
	go func() {
		r, e := auth.Refresh(ctx, login.Refresh, login.CSRFToken, false)
		refreshed <- r
		refreshErr <- e
	}()
	wg.Wait()
	close(results)
	success := 0
	for e := range results {
		if e == nil {
			success++
		} else if !errors.Is(e, usecase.ErrResetToken) {
			t.Fatal(e)
		}
	}
	if success != 1 {
		t.Fatal("concurrent reset successes", success)
	}
	r, e := <-refreshed, <-refreshErr
	if e != nil && !errors.Is(e, usecase.ErrAuth) {
		t.Fatal("refresh/reset race", e)
	}
	if r != nil {
		if _, e = auth.Authenticate(ctx, r.AccessToken); !errors.Is(e, usecase.ErrAuth) {
			t.Fatal("race JWT survives reset")
		}
	}
}

func TestPostgresPasswordRecoveryRotationExpiryAndPersistentQuota(t *testing.T) {
	db := testDB(t)
	migrate(t, db)
	ctx := context.Background()
	repo := storage.NewAuth(db)
	tokens, _ := security.NewTokens(security.Digest("quota jwt"), security.Digest("quota csrf"), "srez", "api")
	auth, v, inbox := testsupport.NewAuth(db, security.NewPasswords(1), tokens)
	u, e := auth.Register(ctx, "reset-quota@example.test", "reset_quota", "Quota", "a sufficiently long password")
	if e != nil {
		t.Fatal(e)
	}
	if e = inbox.Verify(ctx, v, u.Email()); e != nil {
		t.Fatal(e)
	}
	p := usecase.VerificationPolicy{IPMinute: 100, IPHour: 100, AccountHour: 5}
	ip := security.Digest("quota ip")
	first, second := security.Random(), security.Random()
	if _, e = repo.IssuePasswordReset(ctx, u.Email(), security.Digest(first), ip, p); e != nil {
		t.Fatal(e)
	}
	if issued, e := repo.IssuePasswordReset(ctx, u.Email(), security.Digest(second), ip, p); e != nil || issued != nil {
		t.Fatal("cooldown bypass")
	}
	exec(t, db, `UPDATE password_reset_tokens SET created_at=created_at-interval '61 seconds',expires_at=expires_at-interval '61 seconds'`)
	if issued, e := repo.IssuePasswordReset(ctx, u.Email(), security.Digest(second), ip, p); e != nil || issued == nil {
		t.Fatal("resend failed", e)
	}
	if e = repo.ResetPassword(ctx, security.Digest(first), "unused"); !errors.Is(e, usecase.ErrResetToken) {
		t.Fatal("rotated token accepted")
	}
	exec(t, db, `UPDATE password_reset_tokens SET created_at=created_at-interval '31 minutes',expires_at=expires_at-interval '31 minutes' WHERE used_at IS NULL AND revoked_at IS NULL`)
	if e = repo.ResetPassword(ctx, security.Digest(second), "unused"); !errors.Is(e, usecase.ErrResetToken) {
		t.Fatal("expired token accepted")
	}
	for range 3 {
		exec(t, db, `UPDATE password_reset_tokens SET created_at=created_at-interval '61 seconds',expires_at=expires_at-interval '61 seconds',revoked_at=CASE WHEN revoked_at IS NULL THEN NULL ELSE revoked_at-interval '61 seconds' END`)
		if issued, e := repo.IssuePasswordReset(ctx, u.Email(), security.Digest(security.Random()), ip, p); e != nil || issued == nil {
			t.Fatal("hour setup", e)
		}
	}
	exec(t, db, `UPDATE password_reset_tokens SET created_at=created_at-interval '61 seconds',expires_at=expires_at-interval '61 seconds',revoked_at=CASE WHEN revoked_at IS NULL THEN NULL ELSE revoked_at-interval '61 seconds' END`)
	if issued, e := repo.IssuePasswordReset(ctx, u.Email(), security.Digest(security.Random()), ip, p); e != nil || issued != nil {
		t.Fatal("hour cap bypass")
	}
	strict := usecase.VerificationPolicy{IPMinute: 1, IPHour: 2, AccountHour: 5}
	limitedIP := security.Digest("persistent reset ip")
	if _, e = repo.IssuePasswordReset(ctx, "unknown@example.test", security.Digest(security.Random()), limitedIP, strict); e != nil {
		t.Fatal(e)
	}
	if _, e = storage.NewAuth(db).IssuePasswordReset(ctx, "unknown2@example.test", security.Digest(security.Random()), limitedIP, strict); !errors.Is(e, usecase.ErrRecoveryRate) {
		t.Fatal("restart lost quota", e)
	}
}
