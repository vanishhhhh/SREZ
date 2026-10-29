package entities

import (
	"regexp"
	"strings"
	"time"
)

// UserState is a detached persistence snapshot. PasswordHash is never JSON encoded.
type UserState struct {
	ID                           int64
	Email, Username, DisplayName string
	PasswordHash                 string `json:"-"`
	AvatarURL                    string
	CreatedAt, UpdatedAt         time.Time
}

type User struct{ state UserState }

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9_]{3,30}$`)

func NewUser(email, username, displayName, passwordHash string, now time.Time) (*User, error) {
	s := UserState{Email: email, Username: username, DisplayName: strings.TrimSpace(displayName), PasswordHash: passwordHash, CreatedAt: now, UpdatedAt: now}
	if err := validateUser(s); err != nil {
		return nil, err
	}
	return &User{state: s}, nil
}

func RestoreUser(s UserState) (*User, error) {
	if s.ID <= 0 {
		return nil, invalid("user.id")
	}
	s.DisplayName = strings.TrimSpace(s.DisplayName)
	if err := validateUser(s); err != nil {
		return nil, err
	}
	return &User{state: s}, nil
}

func validateUser(s UserState) error {
	if s.ID < 0 {
		return invalid("user.id")
	}
	if !validEmail(s.Email) {
		return invalid("user.email")
	}
	if !usernamePattern.MatchString(s.Username) {
		return invalid("user.username")
	}
	if !nonblankText(s.DisplayName) {
		return invalid("user.display_name")
	}
	if strings.TrimSpace(s.PasswordHash) == "" {
		return invalid("user.password_hash")
	}
	if !validURL(s.AvatarURL, true) {
		return invalid("user.avatar_url")
	}
	if !validTimes(s.CreatedAt, s.UpdatedAt) {
		return invalid("user.timestamps")
	}
	return nil
}

func (u *User) ChangeDisplayName(name string, now time.Time) error {
	s := u.state
	s.DisplayName = strings.TrimSpace(name)
	return u.change(s, now)
}

func (u *User) ChangeAvatar(url string, now time.Time) error {
	s := u.state
	s.AvatarURL = url
	return u.change(s, now)
}

func (u *User) change(s UserState, now time.Time) error {
	if err := checkTime(now, u.state.UpdatedAt); err != nil {
		return err
	}
	s.UpdatedAt = now
	if err := validateUser(s); err != nil {
		return err
	}
	u.state = s
	return nil
}

func (u User) State() UserState    { return u.state }
func (u User) ID() int64           { return u.state.ID }
func (u User) Email() string       { return u.state.Email }
func (u User) Username() string    { return u.state.Username }
func (u User) DisplayName() string { return u.state.DisplayName }

// PasswordHash is for credential verification, not response DTOs.
func (u User) PasswordHash() string { return u.state.PasswordHash }
func (u User) AvatarURL() string    { return u.state.AvatarURL }
func (u User) CreatedAt() time.Time { return u.state.CreatedAt }
func (u User) UpdatedAt() time.Time { return u.state.UpdatedAt }
