package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"srez/internal/entities"
	"srez/internal/security"
)

var ErrAuth = errors.New("authentication failed")
var ErrAccountExists = errors.New("account unavailable")
var ErrRefreshRace = errors.New("refresh already rotated; obtain current cookie")
var ErrCSRF = errors.New("csrf failed")

type AuthRepository interface {
	Register(context.Context, entities.User) (*entities.User, error)
	FindLogin(context.Context, string) (*entities.User, error)
	Rehash(context.Context, int64, string, string) error
	CreateSession(context.Context, entities.AuthSession, []byte, string) error
	AccessSession(context.Context, int64, string) (bool, error)
	Session(context.Context, []byte) (*entities.AuthSession, error)
	Rotate(context.Context, []byte, []byte, string, bool) (*entities.AuthSession, error)
}
type Auth struct {
	repo      AuthRepository
	passwords *security.Passwords
	tokens    *security.Tokens
	email     *EmailVerification
	recovery  *PasswordRecovery
}

func (a *Auth) SetRecovery(r *PasswordRecovery) { a.recovery = r }
func (a *Auth) ForgotPassword(ctx context.Context, email, ip string) error {
	if a.recovery == nil {
		return ErrRecoveryUnavailable
	}
	return a.recovery.Forgot(ctx, email, ip)
}
func (a *Auth) ResetPassword(ctx context.Context, token, password string) error {
	if a.recovery == nil {
		return ErrRecoveryUnavailable
	}
	return a.recovery.Reset(ctx, token, password)
}

// One indexed lookup per authenticated request gives immediate revocation,
// including logout, replay detection and password reset. No stale cache.
func (a *Auth) Authenticate(ctx context.Context, raw string) (int64, error) {
	uid, sid, err := a.tokens.VerifyIdentity(raw)
	if err != nil {
		return 0, ErrAuth
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	active, err := a.repo.AccessSession(ctx, uid, sid)
	if err != nil {
		return 0, err
	}
	if !active {
		return 0, ErrAuth
	}
	return uid, nil
}

func NewAuth(r AuthRepository, p *security.Passwords, t *security.Tokens, email ...*EmailVerification) *Auth {
	a := &Auth{repo: r, passwords: p, tokens: t}
	if len(email) > 0 {
		a.email = email[0]
	}
	return a
}

type AuthResult struct {
	AccessToken string    `json:"access_token"`
	CSRFToken   string    `json:"csrf_token"`
	Refresh     string    `json:"-"`
	ExpiresAt   time.Time `json:"-"`
}

func (a *Auth) Register(ctx context.Context, email, username, name, password string) (*entities.User, error) {
	return a.RegisterFrom(ctx, email, username, name, password, "")
}
func (a *Auth) RegisterFrom(ctx context.Context, email, username, name, password, ip string) (*entities.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if len(email) > 254 {
		return nil, entities.ErrInvalidValue
	}
	for _, r := range email {
		if r > 127 {
			return nil, entities.ErrInvalidValue
		}
	}
	username = strings.ToLower(strings.TrimSpace(username))
	user, err := entities.NewUser(email, username, name, "!", time.Now().UTC())
	if err != nil {
		return nil, err
	}
	hash, err := a.passwords.Hash(ctx, password)
	if err != nil {
		return nil, err
	}
	state := user.State()
	state.PasswordHash = hash
	user, err = entities.NewUser(state.Email, state.Username, state.DisplayName, hash, state.CreatedAt)
	if err != nil {
		return nil, err
	}
	if a.email == nil {
		return nil, ErrVerificationUnavailable
	}
	return a.email.register(ctx, *user, ip)
}
func (a *Auth) VerifyEmail(ctx context.Context, raw string) error {
	if a.email == nil {
		return ErrVerificationUnavailable
	}
	return a.email.Verify(ctx, raw)
}
func (a *Auth) ResendVerification(ctx context.Context, identifier, ip string) error {
	if a.email == nil {
		return ErrVerificationUnavailable
	}
	return a.email.Resend(ctx, identifier, ip)
}
func (a *Auth) Login(ctx context.Context, identifier, password string) (*AuthResult, error) {
	user, err := a.repo.FindLogin(ctx, strings.ToLower(strings.TrimSpace(identifier)))
	if err != nil {
		return nil, err
	}
	hash := "!"
	if user != nil {
		hash = user.PasswordHash()
	}
	ok, rehash, err := a.passwords.Verify(ctx, password, hash)
	if err != nil {
		return nil, err
	}
	if !ok || user == nil {
		return nil, ErrAuth
	}
	// Reveal this state only after verifying the supplied password.
	if !user.CanAuthenticate() {
		return nil, ErrEmailUnverified
	}
	if rehash {
		newHash, e := a.passwords.Upgrade(ctx, password, hash)
		if e != nil {
			return nil, e
		}
		if e = a.repo.Rehash(ctx, user.ID(), hash, newHash); e != nil {
			return nil, e
		}
		hash = newHash
	}
	now := time.Now().UTC()
	session := entities.AuthSession{ID: security.Random(), UserID: user.ID(), CreatedAt: now, ExpiresAt: now.Add(7 * 24 * time.Hour), LastUsedAt: now}
	refresh := security.Random()
	result, err := a.result(session, refresh)
	if err != nil {
		return nil, err
	}
	if err = a.repo.CreateSession(ctx, session, security.Digest(refresh), hash); err != nil {
		return nil, err
	}
	return result, nil
}
func (a *Auth) result(s entities.AuthSession, refresh string) (*AuthResult, error) {
	token, err := a.tokens.Issue(s.UserID, s.ID)
	if err != nil {
		return nil, err
	}
	return &AuthResult{token, a.tokens.CSRF(s.ID), refresh, s.ExpiresAt}, nil
}
func (a *Auth) CSRF(ctx context.Context, refresh string) (string, error) {
	if !security.ValidRandom(refresh) {
		return "", ErrAuth
	}
	s, err := a.repo.Session(ctx, security.Digest(refresh))
	if err != nil {
		return "", err
	}
	if s == nil || s.RefreshUsedAt != nil || !s.Active(time.Now().UTC()) {
		return "", ErrAuth
	}
	return a.tokens.CSRF(s.ID), nil
}
func (a *Auth) Refresh(ctx context.Context, refresh, csrf string, logout bool) (*AuthResult, error) {
	if !security.ValidRandom(refresh) {
		return nil, ErrAuth
	}
	s, err := a.repo.Session(ctx, security.Digest(refresh))
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, ErrAuth
	}
	if !a.tokens.CheckCSRF(s.ID, csrf) {
		return nil, ErrCSRF
	}
	next := security.Random()
	s, err = a.repo.Rotate(ctx, security.Digest(refresh), security.Digest(next), s.ID, logout)
	if err != nil {
		return nil, err
	}
	if logout {
		return nil, nil
	}
	return a.result(*s, next)
}
