// Package security contains bounded password hashing and token adapters.
package security

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/argon2"
)

var ErrInvalid = errors.New("invalid credentials")
var ErrBusy = errors.New("security capacity exhausted")

func Random() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func Digest(s string) []byte { h := sha256.Sum256([]byte(s)); return h[:] }
func ValidRandom(s string) bool {
	b, e := base64.RawURLEncoding.DecodeString(s)
	return e == nil && len(b) == 32 && base64.RawURLEncoding.EncodeToString(b) == s
}
func ValidPassword(s string) bool {
	return utf8.ValidString(s) && utf8.RuneCountInString(s) >= 12 && len(s) <= 1024
}

type Passwords struct {
	slots chan struct{}
	dummy string
}

func NewPasswords(n int) *Passwords {
	p := &Passwords{slots: make(chan struct{}, n)}
	p.dummy = p.hash(Random())
	return p
}
func (p *Passwords) enter(ctx context.Context) error {
	select {
	case p.slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return ErrBusy
	}
}
func (p *Passwords) hash(s string) string { return p.hashWith(s, 19456, 2, 1) }

func (p *Passwords) hashWith(s string, m, t uint32, lanes uint8) string {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		panic("crypto/rand unavailable")
	}
	key := argon2.IDKey([]byte(s), salt, t, m, lanes, 32)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", m, t, lanes, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))
}
func (p *Passwords) Hash(ctx context.Context, s string) (string, error) {
	if !ValidPassword(s) {
		return "", ErrInvalid
	}
	if err := p.enter(ctx); err != nil {
		return "", err
	}
	defer func() { <-p.slots }()
	return p.hash(s), nil
}
func (p *Passwords) Verify(ctx context.Context, s, encoded string) (bool, bool, error) {
	if len(s) > 1024 {
		return false, false, ErrInvalid
	}
	if err := p.enter(ctx); err != nil {
		return false, false, err
	}
	defer func() { <-p.slots }()
	valid := true
	parts := strings.Split(encoded, "$")
	var m, t uint32
	var lanes uint8
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		valid = false
	} else {
		n, e := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &lanes)
		valid = e == nil && n == 3 && m >= 8192 && m <= 65536 && t >= 1 && t <= 4 && lanes >= 1 && lanes <= 4 && parts[3] == fmt.Sprintf("m=%d,t=%d,p=%d", m, t, lanes)
	}
	var salt, want []byte
	if valid {
		var e error
		salt, e = base64.RawStdEncoding.DecodeString(parts[4])
		valid = e == nil && len(salt) == 16
		want, e = base64.RawStdEncoding.DecodeString(parts[5])
		valid = valid && e == nil && len(want) == 32
	}
	if !valid {
		parts = strings.Split(p.dummy, "$")
		salt, _ = base64.RawStdEncoding.DecodeString(parts[4])
		want, _ = base64.RawStdEncoding.DecodeString(parts[5])
		m, t, lanes = 19456, 2, 1
	}
	got := argon2.IDKey([]byte(s), salt, t, m, lanes, 32)
	ok := subtle.ConstantTimeCompare(got, want) == 1 && valid
	return ok, ok && (m < 19456 || t < 2), nil
}

type Claims struct {
	Session string `json:"sid"`
	jwt.RegisteredClaims
}
type Tokens struct {
	key, csrf        []byte
	issuer, audience string
}

func NewTokens(key, csrf []byte, issuer, audience string) (*Tokens, error) {
	if len(key) < 32 || len(csrf) < 32 || hmac.Equal(key, csrf) || issuer == "" || audience == "" {
		return nil, ErrInvalid
	}
	return &Tokens{append([]byte{}, key...), append([]byte{}, csrf...), issuer, audience}, nil
}
func (t *Tokens) Issue(uid int64, sid string) (string, error) {
	now := time.Now().UTC()
	return jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{sid, jwt.RegisteredClaims{Issuer: t.issuer, Audience: jwt.ClaimStrings{t.audience}, Subject: strconv.FormatInt(uid, 10), IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)), ID: Random()}}).SignedString(t.key)
}
func (t *Tokens) Verify(raw string) (int64, error) {
	c := &Claims{}
	_, err := jwt.ParseWithClaims(raw, c, func(token *jwt.Token) (any, error) { return t.key, nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(t.issuer), jwt.WithAudience(t.audience), jwt.WithExpirationRequired(), jwt.WithIssuedAt())
	if err != nil {
		return 0, ErrInvalid
	}
	uid, e := strconv.ParseInt(c.Subject, 10, 64)
	if e != nil || uid <= 0 || strconv.FormatInt(uid, 10) != c.Subject || !ValidRandom(c.Session) || !ValidRandom(c.ID) || c.IssuedAt == nil || c.ExpiresAt == nil || !c.ExpiresAt.After(c.IssuedAt.Time) || c.ExpiresAt.Sub(c.IssuedAt.Time) > 15*time.Minute {
		return 0, ErrInvalid
	}
	return uid, nil
}
func (t *Tokens) CSRF(sid string) string {
	mac := hmac.New(sha256.New, t.csrf)
	mac.Write([]byte("srez:csrf:v1:" + sid))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (t *Tokens) CheckCSRF(sid, token string) bool {
	return hmac.Equal([]byte(t.CSRF(sid)), []byte(token))
}

// Upgrade preserves every supported cost component. Verify validates PHC bounds
// before any old parameters are trusted; work stays under the same semaphore.
func (p *Passwords) Upgrade(ctx context.Context, password, encoded string) (string, error) {
	ok, needed, err := p.Verify(ctx, password, encoded)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrInvalid
	}
	if !needed {
		return encoded, nil
	}
	parts := strings.Split(encoded, "$")
	var m, t uint32
	var lanes uint8
	if _, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &lanes); err != nil {
		return "", ErrInvalid
	}
	if err = p.enter(ctx); err != nil {
		return "", err
	}
	defer func() { <-p.slots }()
	return p.hashWith(password, max(m, 19456), max(t, 2), max(lanes, 1)), nil
}
