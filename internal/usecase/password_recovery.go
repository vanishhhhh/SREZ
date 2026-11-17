package usecase

import (
	"context"
	"errors"
	"net/mail"
	"srez/internal/entities"
	"srez/internal/security"
	"strings"
	"time"
)

var ErrResetToken = errors.New("invalid or expired reset link")
var ErrRecoveryRate = errors.New("password recovery rate limited")
var ErrRecoveryUnavailable = errors.New("password recovery unavailable")

type RecoveryRepository interface {
	IssuePasswordReset(context.Context, string, []byte, []byte, VerificationPolicy) (*entities.User, error)
	ResetPassword(context.Context, []byte, string) error
}
type PasswordRecovery struct {
	repo            RecoveryRepository
	mail            MailSender
	passwords       *security.Passwords
	baseURL         string
	ipKey           []byte
	policy          VerificationPolicy
	slots           chan struct{}
	OnDeliveryError func()
}

func NewPasswordRecovery(repo RecoveryRepository, sender MailSender, passwords *security.Passwords, baseURL string, key []byte, policy VerificationPolicy) *PasswordRecovery {
	return &PasswordRecovery{repo: repo, mail: sender, passwords: passwords, baseURL: strings.TrimRight(baseURL, "/"), ipKey: append([]byte(nil), key...), policy: policy, slots: make(chan struct{}, 8)}
}
func (r *PasswordRecovery) Forgot(ctx context.Context, email, ip string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || len(email) > 254 || strings.ContainsAny(email, "\r\n\x00") {
		return entities.ErrInvalidValue
	}
	for _, char := range email {
		if char > 127 {
			return entities.ErrInvalidValue
		}
	}
	raw := security.Random()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	u, err := r.repo.IssuePasswordReset(ctx, email, security.Digest(raw), challengeIPHash(r.ipKey, ip, "srez-password-reset-ip-v1\x00"), r.policy)
	if err != nil || u == nil {
		return err
	}
	// SMTP runs after commit, without a public timing dependency. No durable
	// plaintext queue: if the process fails, retry after the persisted cooldown.
	go func() {
		select {
		case r.slots <- struct{}{}:
		default:
			if r.OnDeliveryError != nil {
				r.OnDeliveryError()
			}
			return
		}
		defer func() { <-r.slots }()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if r.mail == nil || r.mail.SendPasswordReset(ctx, u.Email(), r.baseURL+"/reset-password#token="+raw) != nil {
			if r.OnDeliveryError != nil {
				r.OnDeliveryError()
			}
		}
	}()
	return nil
}
func (r *PasswordRecovery) Reset(ctx context.Context, raw, password string) error {
	if !security.ValidRandom(raw) {
		return ErrResetToken
	}
	// Use the same bounded Argon2 semaphore as login; hash before SQL locks.
	hash, err := r.passwords.Hash(ctx, password)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return r.repo.ResetPassword(ctx, security.Digest(raw), hash)
}
