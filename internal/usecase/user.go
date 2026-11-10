package usecase

import (
	"context"
	"errors"
	"time"

	"srez/internal/entities"
)

var (
	ErrUserNotFound    = errors.New("user not found")
	ErrUsernameTaken   = errors.New("username already taken")
	ErrEmptyUserUpdate = errors.New("empty user update")
)

type UserRepository interface {
	List(ctx context.Context) ([]entities.User, error)
	// Get returns nil if the ID does not exist.
	Get(ctx context.Context, id int64) (*entities.User, error)
	// Insert returns nil if the username is already taken.
	Insert(ctx context.Context, user entities.User) (*entities.User, error)
	// Update returns nil if the ID does not exist. Nil fields are kept as-is.
	Update(ctx context.Context, id int64, displayName, avatarURL *string, now time.Time) (*entities.User, error)
	Delete(ctx context.Context, id int64) (bool, error)
}

type Users struct {
	repo UserRepository
}

func NewUsers(repo UserRepository) *Users {
	return &Users{repo: repo}
}

func (u *Users) List(ctx context.Context) ([]entities.User, error) {
	return u.repo.List(ctx)
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

func (u *Users) Update(ctx context.Context, id int64, displayName, avatarURL *string) (*entities.User, error) {
	user, err := u.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if displayName == nil && avatarURL == nil {
		return nil, ErrEmptyUserUpdate
	}
	now := time.Now().UTC()
	if now.Before(user.UpdatedAt()) {
		now = user.UpdatedAt()
	}
	if displayName != nil {
		if err := user.ChangeDisplayName(*displayName, now); err != nil {
			return nil, err
		}
		name := user.DisplayName()
		displayName = &name
	}
	if avatarURL != nil {
		if err := user.ChangeAvatar(*avatarURL, now); err != nil {
			return nil, err
		}
	}
	saved, err := u.repo.Update(ctx, id, displayName, avatarURL, now)
	if err != nil {
		return nil, err
	}
	if saved == nil {
		return nil, ErrUserNotFound
	}
	return saved, nil
}

func (u *Users) Delete(ctx context.Context, id int64) error {
	deleted, err := u.repo.Delete(ctx, id)
	if err != nil {
		return err
	}
	if !deleted {
		return ErrUserNotFound
	}
	return nil
}
