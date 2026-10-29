package entities_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"srez/internal/entities"
)

func newUser(t *testing.T) *entities.User {
	t.Helper()
	u, err := entities.NewUser("user@example.com", "user_1", "Пользователь", "secret-hash", created)
	noError(t, err)
	return u
}

func TestNewUser(t *testing.T) {
	for _, tt := range []struct {
		name, email, username, display, hash string
		now                                  time.Time
		valid                                bool
	}{
		{"minimum", "u@example.com", "a_1", "Я", "hash", created, true},
		{"maximum", "u+tag@example.com", strings.Repeat("A", 30), "Имя", "hash", created, true},
		{"short username", "u@example.com", "ab", "Имя", "hash", created, false},
		{"long username", "u@example.com", strings.Repeat("a", 31), "Имя", "hash", created, false},
		{"unicode username", "u@example.com", "юзер", "Имя", "hash", created, false},
		{"username dash", "u@example.com", "user-name", "Имя", "hash", created, false},
		{"email", "invalid", "user", "Имя", "hash", created, false},
		{"email display name", "Name <u@example.com>", "user", "Имя", "hash", created, false},
		{"email newline", "u@example.com\r\n", "user", "Имя", "hash", created, false},
		{"empty email", "", "user", "Имя", "hash", created, false},
		{"display", "u@example.com", "user", " \t", "hash", created, false},
		{"invalid UTF8 display", "u@example.com", "user", "name\xff", "hash", created, false},
		{"hash", "u@example.com", "user", "Имя", " ", created, false},
		{"time", "u@example.com", "user", "Имя", "hash", time.Time{}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			u, err := entities.NewUser(tt.email, tt.username, tt.display, tt.hash, tt.now)
			if !tt.valid {
				if err == nil || u != nil {
					t.Fatalf("got %v, %v", u, err)
				}
				return
			}
			noError(t, err)
			same(t, u.State(), entities.UserState{Email: tt.email, Username: tt.username, DisplayName: tt.display, PasswordHash: tt.hash, CreatedAt: tt.now, UpdatedAt: tt.now})
			same(t, u.ID(), int64(0))
			same(t, u.Email(), tt.email)
			same(t, u.Username(), tt.username)
			same(t, u.DisplayName(), tt.display)
			same(t, u.PasswordHash(), tt.hash)
			same(t, u.AvatarURL(), "")
			same(t, u.CreatedAt(), tt.now)
			same(t, u.UpdatedAt(), tt.now)
		})
	}
}

func TestUserChanges(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*entities.User) error
		valid  bool
	}{
		{"display", func(u *entities.User) error { return u.ChangeDisplayName("Новое имя", changed) }, true},
		{"avatar", func(u *entities.User) error { return u.ChangeAvatar("https://example.com/a.png", changed) }, true},
		{"clear avatar", func(u *entities.User) error { return u.ChangeAvatar("", changed) }, true},
		{"empty display", func(u *entities.User) error { return u.ChangeDisplayName(" ", changed) }, false},
		{"http", func(u *entities.User) error { return u.ChangeAvatar("http://example.com/a", changed) }, false},
		{"relative", func(u *entities.User) error { return u.ChangeAvatar("/a", changed) }, false},
		{"missing host", func(u *entities.User) error { return u.ChangeAvatar("https:///a", changed) }, false},
		{"credentials", func(u *entities.User) error { return u.ChangeAvatar("https://u:p@example.com/a", changed) }, false},
		{"old time", func(u *entities.User) error { return u.ChangeDisplayName("Новое", created.Add(-time.Second)) }, false},
		{"zero time", func(u *entities.User) error { return u.ChangeAvatar("", time.Time{}) }, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			u := newUser(t)
			if tt.name == "clear avatar" {
				noError(t, u.ChangeAvatar("https://example.com/old.png", created))
			}
			before := u.State()
			err := tt.change(u)
			if !tt.valid {
				unchanged(t, before, u.State(), err, entities.ErrInvalidValue)
				return
			}
			noError(t, err)
			same(t, u.UpdatedAt(), changed)
			if tt.name == "display" {
				same(t, u.DisplayName(), "Новое имя")
			}
			if tt.name == "avatar" {
				same(t, u.AvatarURL(), "https://example.com/a.png")
			}
			if tt.name == "clear avatar" {
				same(t, u.AvatarURL(), "")
			}
		})
	}
}

func TestUserRestoreAndJSON(t *testing.T) {
	s := newUser(t).State()
	s.ID = 42
	u, err := entities.RestoreUser(s)
	noError(t, err)
	same(t, u.ID(), int64(42))
	s.DisplayName = "External"
	same(t, u.DisplayName(), "Пользователь")
	for _, value := range []any{u, u.State()} {
		data, err := json.Marshal(value)
		noError(t, err)
		if strings.Contains(strings.ToLower(string(data)), "hash") {
			t.Fatalf("password exposed: %s", data)
		}
	}
	for _, id := range []int64{0, -1} {
		s.ID = id
		_, err := entities.RestoreUser(s)
		if err == nil {
			t.Fatal("invalid ID accepted")
		}
	}
	s.ID = 42
	s.UpdatedAt = created.Add(-time.Second)
	_, err = entities.RestoreUser(s)
	if err == nil {
		t.Fatal("invalid timestamps accepted")
	}
}

func TestUserDisplayNameNormalization(t *testing.T) {
	for _, input := range []string{" Имя ", "\tИмя\n", "\u00a0Имя\u00a0"} {
		t.Run(input, func(t *testing.T) {
			u, err := entities.NewUser("u@example.com", "user", input, " hash ", created)
			noError(t, err)
			same(t, u.DisplayName(), "Имя")
			same(t, u.PasswordHash(), " hash ")
			s := u.State()
			s.ID = 1
			s.DisplayName = input
			u, err = entities.RestoreUser(s)
			noError(t, err)
			same(t, u.DisplayName(), "Имя")
			noError(t, u.ChangeDisplayName(" Новое имя ", changed))
			same(t, u.DisplayName(), "Новое имя")
			before := u.State()
			err = u.ChangeDisplayName("\u00a0\t ", changed.Add(time.Hour))
			unchanged(t, before, u.State(), err, entities.ErrInvalidValue)
		})
	}
}
