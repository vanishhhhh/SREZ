package security

import (
	"bytes"
	"github.com/golang-jwt/jwt/v5"
	"testing"
	"time"
)

func TestIssuedJWTHandlesSubsecondClockStepWithoutExpiryLeeway(t *testing.T) {
	key := bytes.Repeat([]byte{11}, 32)
	tokens, err := NewTokens(key, bytes.Repeat([]byte{12}, 32), "clock", "clock-api")
	if err != nil {
		t.Fatal(err)
	}
	wall := time.Unix(1800000000, 50000000).UTC()
	raw, err := tokens.issueAt(7, Random(), wall)
	if err != nil {
		t.Fatal(err)
	}
	parse := func(at time.Time) (*Claims, error) {
		c := &Claims{}
		_, e := jwt.ParseWithClaims(raw, c, func(*jwt.Token) (any, error) { return key, nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer("clock"), jwt.WithAudience("clock-api"), jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithTimeFunc(func() time.Time { return at }))
		return c, e
	}
	c, err := parse(wall.Add(-200 * time.Millisecond))
	if err != nil {
		t.Fatal("own token rejected after a bounded clock step")
	}
	if c.ExpiresAt.Sub(c.IssuedAt.Time) != 15*time.Minute || c.ExpiresAt.After(wall.Add(15*time.Minute)) {
		t.Fatal("access lifetime extended")
	}
	if _, err = parse(c.ExpiresAt.Time); err == nil {
		t.Fatal("expired token accepted")
	}
	if _, err = parse(c.IssuedAt.Add(-time.Millisecond)); err == nil {
		t.Fatal("future issued-at accepted")
	}
}
