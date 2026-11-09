package usecase

import (
	"context"
	"errors"
	"time"

	"srez/internal/entities"
)

var (
	ErrUserNotFound  = errors.New("user not found")
	ErrUsernameTaken = errors.New("username already taken")
)

type UserRepository interface {
	// Get returns nil if the ID does not exist.
	Get(ctx context.Context, id int64) (*entities.User, error)
	// Insert returns nil if the username is already taken.
	Insert(ctx context.Context, user entities.User) (*entities.User, error)
}

type Users struct {
	repo UserRepository
}

func NewUsers(repo UserRepository) *Users {
	return &Users{repo: repo}
}

func (u *Users) Get(ctx context.Context, id int64) (*entities.User, error) {
	user, err := u.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound
	}
	return user, nil
}

func (u *Users) Create(ctx context.Context, email, username, displayName string) (*entities.User, error) {
	// The account record starts without a usable password hash.
	user, err := entities.NewUser(email, username, displayName, "!", time.Now().UTC())
	if err != nil {
		return nil, err
	}
	saved, err := u.repo.Insert(ctx, *user)
	if err != nil {
		return nil, err
	}
	if saved == nil {
		return nil, ErrUsernameTaken
	}
	return saved, nil
}
