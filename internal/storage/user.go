package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"

	"srez/internal/entities"
)

type Users struct {
	db *sqlx.DB
}

func NewUsers(db *sqlx.DB) *Users {
	return &Users{db: db}
}

type userRow struct {
	ID           int64     `db:"id"`
	Email        string    `db:"email"`
	Username     string    `db:"username"`
	DisplayName  string    `db:"display_name"`
	PasswordHash string    `db:"password_hash"`
	AvatarURL    string    `db:"avatar_url"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}

func (r userRow) user() (*entities.User, error) {
	return entities.RestoreUser(entities.UserState{
		ID: r.ID, Email: r.Email, Username: r.Username, DisplayName: r.DisplayName,
		PasswordHash: r.PasswordHash, AvatarURL: r.AvatarURL, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	})
}

const userColumns = "id, email, username, display_name, password_hash, avatar_url, created_at, updated_at"

func (s *Users) List(ctx context.Context) ([]entities.User, error) {
	var rows []userRow
	if err := s.db.SelectContext(ctx, &rows, `SELECT `+userColumns+` FROM users ORDER BY id`); err != nil {
		return nil, err
	}
	users := make([]entities.User, 0, len(rows))
	for _, row := range rows {
		user, err := row.user()
		if err != nil {
			return nil, err
		}
		users = append(users, *user)
	}
	return users, nil
}

func (s *Users) Get(ctx context.Context, id int64) (*entities.User, error) {
	var row userRow
	err := s.db.GetContext(ctx, &row, `SELECT `+userColumns+` FROM users WHERE id = $1`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return row.user()
}

func (s *Users) Insert(ctx context.Context, user entities.User) (*entities.User, error) {
	const query = `INSERT INTO users (email, username, display_name, password_hash, avatar_url, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (username) DO NOTHING
		RETURNING ` + userColumns
	var row userRow
	err := s.db.GetContext(ctx, &row, query, user.Email(), user.Username(), user.DisplayName(),
		user.PasswordHash(), user.AvatarURL(), user.CreatedAt(), user.UpdatedAt())
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return row.user()
}

func (s *Users) Update(ctx context.Context, id int64, displayName, avatarURL *string, now time.Time) (*entities.User, error) {
	const query = `UPDATE users
		SET display_name = COALESCE($2, display_name),
		    avatar_url = COALESCE($3, avatar_url),
		    updated_at = GREATEST(updated_at, $4)
		WHERE id = $1
		RETURNING ` + userColumns
	var row userRow
	err := s.db.GetContext(ctx, &row, query, id, displayName, avatarURL, now)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return row.user()
}

func (s *Users) Delete(ctx context.Context, id int64) (bool, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count > 0, err
}
