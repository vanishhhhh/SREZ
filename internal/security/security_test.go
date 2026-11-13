package security

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func testTokens(t *testing.T) *Tokens {
	t.Helper()
	x, e := NewTokens([]byte(strings.Repeat("a", 32)), []byte(strings.Repeat("b", 32)), "issuer", "audience")
	if e != nil {
		t.Fatal(e)
	}
	return x
}
func TestPasswords(t *testing.T) {
	p := NewPasswords(1)
	ctx := context.Background()
	password := "a sufficiently long password"
	a, e := p.Hash(ctx, password)
	if e != nil {
		t.Fatal(e)
	}
	b, _ := p.Hash(ctx, password)
	if a == b {
		t.Fatal("salt reused")
	}
	ok, _, e := p.Verify(ctx, password, a)
	if !ok || e != nil {
		t.Fatal(e)
	}
	for _, hash := range []string{"!", "broken", a} {
		ok, _, _ := p.Verify(ctx, "wrong", hash)
		if ok {
			t.Fatal("wrong password accepted")
		}
	}
	for _, s := range []string{"short", strings.Repeat("a", 1025)} {
		if _, e := p.Hash(ctx, s); e == nil {
			t.Fatal("invalid password accepted")
		}
	}
	p.slots <- struct{}{}
	if _, e := p.Hash(ctx, password); e != ErrBusy {
		t.Fatal(e)
	}
	<-p.slots
}
func TestJWT(t *testing.T) {
	x := testTokens(t)
	s := Random()
	good, e := x.Issue(42, s)
	if e != nil {
		t.Fatal(e)
	}
	if id, e := x.Verify(good); id != 42 || e != nil {
		t.Fatal(id, e)
	}
	for _, name := range []string{"expired", "issuer", "audience", "missing_iat", "missing_exp", "missing_jti", "missing_issuer", "missing_audience", "missing_subject", "subject", "sid", "future", "long_ttl", "signature", "none", "hs384"} {
		t.Run(name, func(t *testing.T) {
			now := time.Now()
			c := Claims{s, jwt.RegisteredClaims{Issuer: "issuer", Audience: jwt.ClaimStrings{"audience"}, Subject: "42", IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)), ID: Random()}}
			method := jwt.SigningMethod(jwt.SigningMethodHS256)
			key := any(x.key)
			switch name {
			case "expired":
				c.ExpiresAt = jwt.NewNumericDate(now.Add(-time.Minute))
			case "issuer":
				c.Issuer = "bad"
			case "audience":
				c.Audience = jwt.ClaimStrings{"bad"}
			case "missing_issuer":
				c.Issuer = ""
			case "missing_audience":
				c.Audience = nil
			case "missing_subject":
				c.Subject = ""
			case "missing_iat":
				c.IssuedAt = nil
			case "missing_exp":
				c.ExpiresAt = nil
			case "missing_jti":
				c.ID = ""
			case "subject":
				c.Subject = "-1"
			case "sid":
				c.Session = ""
			case "future":
				c.IssuedAt = jwt.NewNumericDate(now.Add(time.Hour))
			case "long_ttl":
				c.ExpiresAt = jwt.NewNumericDate(now.Add(time.Hour))
			case "signature":
				key = []byte(strings.Repeat("z", 32))
			case "none":
				method = jwt.SigningMethodNone
				key = jwt.UnsafeAllowNoneSignatureType
			case "hs384":
				method = jwt.SigningMethodHS384
			}
			raw, e := jwt.NewWithClaims(method, c).SignedString(key)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = x.Verify(raw); e == nil {
				t.Fatal("invalid jwt accepted")
			}
		})
	}
	if x.CheckCSRF(Random(), x.CSRF(s)) {
		t.Fatal("csrf cross-session accepted")
	}
}

func TestUpgradePreservesMixedParameters(t *testing.T) {
	p := NewPasswords(1)
	ctx := context.Background()
	pw := "mixed parameters test password"
	for _, cost := range []struct {
		m, t uint32
		p    uint8
	}{{65536, 1, 1}, {8192, 4, 2}, {19456, 2, 1}, {65536, 4, 4}} {
		old := p.hashWith(pw, cost.m, cost.t, cost.p)
		next, e := p.Upgrade(ctx, pw, old)
		if e != nil {
			t.Fatal(e)
		}
		prefix := fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$", max(cost.m, 19456), max(cost.t, 2), cost.p)
		if !strings.HasPrefix(next, prefix) {
			t.Fatal("cost component decreased")
		}
		if cost.m >= 19456 && cost.t >= 2 {
			if next != old {
				t.Fatal("unnecessary rehash")
			}
		} else if strings.Split(old, "$")[4] == strings.Split(next, "$")[4] {
			t.Fatal("salt reused")
		}
		if ok, needed, e := p.Verify(ctx, pw, next); e != nil || !ok || needed {
			t.Fatal("invalid upgrade")
		}
	}
	if _, e := p.Upgrade(ctx, "incorrect", p.hash(pw)); e == nil {
		t.Fatal("wrong password accepted")
	}
	if _, e := p.Upgrade(ctx, pw, "$argon2id$v=19$m=999999999,t=2,p=1$x$y"); e == nil {
		t.Fatal("unbounded parameters accepted")
	}
}
