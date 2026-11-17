package usecase

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"net"
	"strings"
	"time"

	"srez/internal/entities"
	"srez/internal/security"
)

var ErrEmailUnverified = errors.New("email verification required")
var ErrVerificationToken = errors.New("invalid or expired verification link")
var ErrVerificationRate = errors.New("email verification rate limited")
var ErrMailDelivery = errors.New("verification email delivery unavailable")
var ErrVerificationUnavailable = errors.New("email verification unavailable")

type VerificationPolicy struct{ IPMinute, IPHour, AccountHour int }

func DefaultVerificationPolicy() VerificationPolicy { return VerificationPolicy{10, 50, 5} }

// MailSender receives a link only in memory. Implementations must not log it.
type MailSender interface {
	SendVerification(context.Context, string, string) error
	SendPasswordReset(context.Context, string, string) error
}
type EmailVerificationRepository interface {
	RegisterUnverified(context.Context, entities.User, []byte, []byte, VerificationPolicy) (*entities.User, error)
	RotateVerification(context.Context, string, []byte, []byte, VerificationPolicy) (*entities.User, error)
	VerifyEmail(context.Context, []byte) error
}
type EmailVerification struct {
	repo            EmailVerificationRepository
	mail            MailSender
	baseURL         string
	ipKey           []byte
	policy          VerificationPolicy
	slots           chan struct{}
	OnDeliveryError func()
}

func NewEmailVerification(repo EmailVerificationRepository, mail MailSender, baseURL string, ipKey []byte, policy VerificationPolicy) *EmailVerification {
	return &EmailVerification{repo: repo, mail: mail, baseURL: strings.TrimRight(baseURL, "/"), ipKey: append([]byte(nil), ipKey...), policy: policy, slots: make(chan struct{}, 8)}
}
func (v *EmailVerification) ipHash(ip string) []byte {
	return challengeIPHash(v.ipKey, ip, "srez-email-ip-v1\x00")
}
func challengeIPHash(key []byte, ip, purpose string) []byte {
	if p := net.ParseIP(ip); p != nil {
		ip = p.String()
	} else {
		ip = "unknown"
	}
	h := hmac.New(sha256.New, key)
	h.Write([]byte(purpose))
	h.Write([]byte(ip))
	return h.Sum(nil)
}
func (v *EmailVerification) deliver(ctx context.Context, email, raw string) error {
	if v.mail == nil {
		return ErrMailDelivery
	}
	select {
	case v.slots <- struct{}{}:
	default:
		return ErrMailDelivery
	}
	defer func() { <-v.slots }()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := v.mail.SendVerification(ctx, email, v.baseURL+"/verify-email#token="+raw); err != nil {
		return ErrMailDelivery
	}
	return nil
}
func (v *EmailVerification) register(ctx context.Context, u entities.User, ip string) (*entities.User, error) {
	raw := security.Random()
	saved, err := v.repo.RegisterUnverified(ctx, u, security.Digest(raw), v.ipHash(ip), v.policy)
	if err != nil {
		return nil, err
	}
	// Commit has completed before any network operation starts.
	return saved, v.deliver(ctx, saved.Email(), raw)
}
func (v *EmailVerification) Resend(ctx context.Context, identifier, ip string) error {
	identifier = strings.ToLower(strings.TrimSpace(identifier))
	if identifier == "" || len(identifier) > 254 || strings.ContainsAny(identifier, "\r\n\x00") {
		return entities.ErrInvalidValue
	}
	for _, r := range identifier {
		if r > 127 {
			return entities.ErrInvalidValue
		}
	}
	raw := security.Random()
	user, err := v.repo.RotateVerification(ctx, identifier, security.Digest(raw), v.ipHash(ip), v.policy)
	if err != nil || user == nil {
		return err
	}
	// Response never claims delivery and is independent of SMTP timing/account
	// existence. No durable plaintext queue; a crash is recovered via resend.
	go func() {
		if err := v.deliver(context.Background(), user.Email(), raw); err != nil && v.OnDeliveryError != nil {
			v.OnDeliveryError()
		}
	}()
	return nil
}
func (v *EmailVerification) Verify(ctx context.Context, raw string) error {
	if !security.ValidRandom(raw) {
		return ErrVerificationToken
	}
	return v.repo.VerifyEmail(ctx, security.Digest(raw))
}
