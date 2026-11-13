package storage_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"srez/internal/security"
	"srez/internal/storage"
	"srez/internal/usecase"
	"srez/migrations"
)

func TestPostgresAuth(t *testing.T) {
	db := testDB(t)
	migrate(t, db)
	ctx := context.Background()
	repo := storage.NewAuth(db)
	tokens, _ := security.NewTokens([]byte(strings.Repeat("a", 32)), []byte(strings.Repeat("b", 32)), "srez", "api")
	auth := usecase.NewAuth(repo, security.NewPasswords(2), tokens)
	pw := "long enough password 123"
	u, e := auth.Register(ctx, "Alice@EXAMPLE.com", "ALICE", "Alice", pw)
	if e != nil {
		t.Fatal(e)
	}
	if u.Email() != "alice@example.com" || u.Username() != "alice" || !strings.HasPrefix(u.PasswordHash(), "$argon2id$") {
		t.Fatal("normalization/hash")
	}
	for _, pair := range [][2]string{{"alice@example.com", "another"}, {"other@example.com", "alice"}} {
		if _, e = auth.Register(ctx, pair[0], pair[1], "Test", pw); !errors.Is(e, usecase.ErrAccountExists) {
			t.Fatal(e)
		}
	}
	exec(t, db, `INSERT INTO users(email,username,display_name,password_hash,created_at,updated_at) VALUES('legacy@example.com','legacy','Legacy','!',now(),now())`)
	if _, e = auth.Register(ctx, "LEGACY@example.com", "another", "Test", pw); !errors.Is(e, usecase.ErrAccountExists) {
		t.Fatal("legacy takeover", e)
	}
	for _, key := range []string{"legacy", "missing", "alice"} {
		if _, e = auth.Login(ctx, key, "incorrect"); !errors.Is(e, usecase.ErrAuth) {
			t.Fatal(e)
		}
	}
	login, e := auth.Login(ctx, "ALICE", pw)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = tokens.Verify(login.AccessToken); e != nil {
		t.Fatal(e)
	}
	csrf, e := auth.CSRF(ctx, login.Refresh)
	if e != nil || csrf != login.CSRFToken {
		t.Fatal("csrf reconstruction", e)
	}
	if _, e = auth.Refresh(ctx, login.Refresh, "wrong", false); !errors.Is(e, usecase.ErrCSRF) {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	results := make(chan *usecase.AuthResult, 2)
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := auth.Refresh(ctx, login.Refresh, csrf, false)
			results <- r
			errs <- e
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	var rotated *usecase.AuthResult
	success, races := 0, 0
	for r := range results {
		if r != nil {
			rotated = r
			success++
		}
	}
	for e := range errs {
		if errors.Is(e, usecase.ErrRefreshRace) {
			races++
		} else if e != nil {
			t.Fatal(e)
		}
	}
	if success != 1 || races != 1 {
		t.Fatal(success, races)
	}
	if login.ExpiresAt.Sub(rotated.ExpiresAt) > time.Microsecond || rotated.ExpiresAt.Sub(login.ExpiresAt) > time.Microsecond {
		t.Fatal("absolute expiry extended")
	}
	exec(t, db, `UPDATE auth_refresh_tokens SET used_at=now()-interval '10 seconds' WHERE used_at IS NOT NULL`)
	if _, e = auth.Refresh(ctx, login.Refresh, csrf, false); !errors.Is(e, usecase.ErrAuth) {
		t.Fatal(e)
	}
	if _, e = auth.Refresh(ctx, rotated.Refresh, csrf, false); !errors.Is(e, usecase.ErrAuth) {
		t.Fatal("replay did not revoke", e)
	}
	fresh, e := auth.Login(ctx, "alice", pw)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = auth.Refresh(ctx, fresh.Refresh, fresh.CSRFToken, true); e != nil {
		t.Fatal(e)
	}
	if _, e = auth.Refresh(ctx, fresh.Refresh, fresh.CSRFToken, true); e != nil {
		t.Fatal("logout not idempotent", e)
	}
	if _, e = auth.Refresh(ctx, fresh.Refresh, fresh.CSRFToken, false); !errors.Is(e, usecase.ErrAuth) {
		t.Fatal(e)
	}
}
func TestPostgresAuthMigrationCollision(t *testing.T) {
	db := testDB(t)
	body := `CREATE TABLE users(id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,email TEXT NOT NULL,username TEXT NOT NULL UNIQUE,display_name TEXT NOT NULL,password_hash TEXT NOT NULL,avatar_url TEXT NOT NULL DEFAULT '',created_at TIMESTAMPTZ NOT NULL,updated_at TIMESTAMPTZ NOT NULL); INSERT INTO users(email,username,display_name,password_hash,created_at,updated_at) VALUES('Same@example.com','First','First','!',now(),now()),('same@example.com','Second','Second','!',now(),now())`
	exec(t, db, body)
	if e := migrations.Apply(context.Background(), db); e == nil {
		t.Fatal("conflicts silently accepted")
	}
	if count(t, db, "users") != 2 {
		t.Fatal("legacy users changed")
	}
}

func TestPostgresRegistrationRace(t *testing.T) {
	db := testDB(t)
	migrate(t, db)
	ctx := context.Background()
	repo := storage.NewAuth(db)
	tokens, _ := security.NewTokens([]byte(strings.Repeat("a", 32)), []byte(strings.Repeat("b", 32)), "srez", "api")
	auth := usecase.NewAuth(repo, security.NewPasswords(2), tokens)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := auth.Register(ctx, "race@example.com", "race_user", "Race", "a secure long password")
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	ok, conflict := 0, 0
	for e := range errs {
		if e == nil {
			ok++
		} else if errors.Is(e, usecase.ErrAccountExists) {
			conflict++
		} else {
			t.Fatal(e)
		}
	}
	if ok != 1 || conflict != 1 || count(t, db, "users") != 1 {
		t.Fatal(ok, conflict)
	}
}

func TestPostgresExpiredAndSessionCSRF(t *testing.T) {
	db := testDB(t)
	migrate(t, db)
	ctx := context.Background()
	tokens, _ := security.NewTokens([]byte(strings.Repeat("a", 32)), []byte(strings.Repeat("b", 32)), "srez", "api")
	auth := usecase.NewAuth(storage.NewAuth(db), security.NewPasswords(1), tokens)
	pw := "a secure long password"
	if _, e := auth.Register(ctx, "expiry@example.com", "expiry", "Expiry", pw); e != nil {
		t.Fatal(e)
	}
	one, e := auth.Login(ctx, "expiry", pw)
	if e != nil {
		t.Fatal(e)
	}
	two, e := auth.Login(ctx, "expiry", pw)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = auth.Refresh(ctx, one.Refresh, two.CSRFToken, false); !errors.Is(e, usecase.ErrCSRF) {
		t.Fatal(e)
	}
	exec(t, db, `UPDATE auth_sessions SET created_at=now()-interval '8 days',expires_at=now()-interval '1 day'`)
	if _, e = auth.Refresh(ctx, one.Refresh, one.CSRFToken, false); !errors.Is(e, usecase.ErrAuth) {
		t.Fatal(e)
	}
	if _, e = auth.CSRF(ctx, one.Refresh); !errors.Is(e, usecase.ErrAuth) {
		t.Fatal(e)
	}
}
