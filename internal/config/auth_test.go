package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func authEnv(t *testing.T) {
	t.Helper()
	t.Setenv("SREZ_JWT_KEY", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 32))))
	t.Setenv("SREZ_CSRF_KEY", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("b", 32))))
	t.Setenv("SREZ_JWT_ISSUER", "srez")
	t.Setenv("SREZ_JWT_AUDIENCE", "api")
	t.Setenv("SREZ_PUBLIC_ORIGIN", "https://example.com")
	t.Setenv("SREZ_AUTH_LOCAL_HTTP", "false")
	t.Setenv("SREZ_AUTH_ATTEMPTS", "10")
	t.Setenv("SREZ_AUTH_HASH_WORKERS", "2")
}
func TestAuthConfig(t *testing.T) {
	authEnv(t)
	if _, e := LoadAuth(); e != nil {
		t.Fatal(e)
	}
	for _, tt := range []struct{ k, v string }{{"SREZ_JWT_KEY", ""}, {"SREZ_CSRF_KEY", "broken"}, {"SREZ_PUBLIC_ORIGIN", "http://example.com"}, {"SREZ_PUBLIC_ORIGIN", "https://example.com/path"}, {"SREZ_JWT_ISSUER", ""}, {"SREZ_AUTH_HASH_WORKERS", "9"}, {"SREZ_AUTH_ATTEMPTS", "0"}} {
		t.Run(tt.k+tt.v, func(t *testing.T) {
			t.Setenv(tt.k, tt.v)
			if _, e := LoadAuth(); e == nil {
				t.Fatal("unsafe config accepted")
			}
		})
	}
	t.Setenv("SREZ_AUTH_LOCAL_HTTP", "true")
	t.Setenv("SREZ_PUBLIC_ORIGIN", "http://127.0.0.1:8080")
	if _, e := LoadAuth(); e != nil {
		t.Fatal(e)
	}
	t.Setenv("SREZ_PUBLIC_ORIGIN", "http://public.example.com")
	if _, e := LoadAuth(); e == nil {
		t.Fatal("insecure public cookies")
	}
}
