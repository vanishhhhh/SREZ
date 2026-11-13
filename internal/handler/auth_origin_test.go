package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"srez/internal/config"
	"srez/internal/entities"
	"srez/internal/security"
	"srez/internal/usecase"
)

type csrfReadRepository struct {
	noAuthRepo
	session       entities.AuthSession
	hash          []byte
	reads, writes int
}

func (r *csrfReadRepository) Session(_ context.Context, hash []byte) (*entities.AuthSession, error) {
	r.reads++
	if !bytes.Equal(hash, r.hash) {
		return nil, nil
	}
	s := r.session
	return &s, nil
}
func (r *csrfReadRepository) Rotate(context.Context, []byte, []byte, string, bool) (*entities.AuthSession, error) {
	r.writes++
	return nil, usecase.ErrAuth
}

func TestCSRFGetSourcePolicy(t *testing.T) {
	const origin = "http://127.0.0.1:8080"
	refresh := security.Random()
	now := time.Now().UTC()
	for _, tt := range []struct {
		name, origin, site, cookie string
		status                     int
	}{
		{"browser same-origin without Origin", "", "same-origin", refresh, 200},
		{"legacy client without source headers", "", "", refresh, 200},
		{"matching Origin", origin, "", refresh, 200},
		{"matching Origin and metadata", origin, "same-origin", refresh, 200},
		{"foreign Origin", "https://evil.example", "same-origin", refresh, 403},
		{"opaque Origin", "null", "", refresh, 403},
		{"cross-site without Origin", "", "cross-site", refresh, 403},
		{"cross-site with matching Origin", origin, "cross-site", refresh, 403},
		{"same-site is not same-origin", "", "same-site", refresh, 403},
		{"navigation none", "", "none", refresh, 403},
		{"unknown metadata", "", "unknown", refresh, 403},
		{"missing cookie", "", "same-origin", "", 401},
		{"unknown cookie", "", "same-origin", security.Random(), 401},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := &csrfReadRepository{session: entities.AuthSession{ID: security.Random(), UserID: 1, CreatedAt: now, LastUsedAt: now, ExpiresAt: now.Add(time.Hour)}, hash: security.Digest(refresh)}
			before := repo.session
			_, tokens := secureHandler(t)
			h := NewWithUsers(usecase.NewUsers(mockUserRepository{}))
			if err := h.EnableAuth(usecase.NewAuth(repo, nil, tokens), tokens, config.Auth{Origin: origin, LocalHTTP: true, Attempts: 10}); err != nil {
				t.Fatal(err)
			}
			// Repeated GET must return the same session-bound token without writes.
			for range 2 {
				req := httptest.NewRequest("GET", "/auth/csrf", nil)
				if tt.origin != "" {
					req.Header.Set("Origin", tt.origin)
				}
				if tt.site != "" {
					req.Header.Set("Sec-Fetch-Site", tt.site)
				}
				if tt.cookie != "" {
					req.AddCookie(&http.Cookie{Name: "srez_refresh_local", Value: tt.cookie})
				}
				res := httptest.NewRecorder()
				h.router.ServeHTTP(res, req)
				if res.Code != tt.status {
					t.Fatalf("status=%d want=%d", res.Code, tt.status)
				}
				if res.Header().Get("Cache-Control") != "no-store" {
					t.Fatal("cache enabled")
				}
				for _, header := range []string{"Access-Control-Allow-Origin", "Access-Control-Allow-Credentials", "Set-Cookie"} {
					if res.Header().Get(header) != "" {
						t.Fatalf("unexpected %s", header)
					}
				}
				if tt.status == 200 {
					assertCatalogJSON(t, res.Body.Bytes(), `{"csrf_token":"`+tokens.CSRF(repo.session.ID)+`"}`)
				}
			}
			if repo.writes != 0 || repo.session != before {
				t.Fatal("GET changed session")
			}
			if tt.status == 403 && repo.reads != 0 {
				t.Fatal("source rejection reached repository")
			}
		})
	}
}

func TestAuthPostStillRequiresOrigin(t *testing.T) {
	for _, path := range []string{"/auth/register", "/auth/login", "/auth/refresh", "/auth/logout"} {
		for _, origin := range []string{"", "https://evil.example", "null"} {
			t.Run(path+origin, func(t *testing.T) {
				h, _ := secureHandler(t)
				req := httptest.NewRequest("POST", path, bytes.NewBufferString(`{}`))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Sec-Fetch-Site", "same-origin")
				if origin != "" {
					req.Header.Set("Origin", origin)
				}
				res := httptest.NewRecorder()
				h.router.ServeHTTP(res, req)
				if res.Code != 403 {
					t.Fatalf("status=%d", res.Code)
				}
			})
		}
	}
}

func TestAuthPostMatchingOriginAndFetchMetadata(t *testing.T) {
	for _, tt := range []struct {
		path   string
		status int
	}{{"/auth/register", 400}, {"/auth/login", 401}, {"/auth/refresh", 401}, {"/auth/logout", 401}} {
		for _, site := range []string{"same-origin", "cross-site"} {
			t.Run(tt.path+site, func(t *testing.T) {
				h, _ := secureHandler(t)
				req := httptest.NewRequest("POST", tt.path, bytes.NewBufferString(`{}`))
				req.Header.Set("Origin", "http://127.0.0.1:8080")
				req.Header.Set("Sec-Fetch-Site", site)
				req.Header.Set("Content-Type", "application/json")
				res := httptest.NewRecorder()
				h.router.ServeHTTP(res, req)
				want := tt.status
				if site == "cross-site" {
					want = 403
				}
				if res.Code != want {
					t.Fatalf("status=%d want=%d", res.Code, want)
				}
			})
		}
	}
}
