package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"

	"srez/internal/entities"
	"srez/internal/usecase"
)

type Auth struct{ db *sqlx.DB }

func NewAuth(db *sqlx.DB) *Auth { return &Auth{db} }
func (a *Auth) Register(ctx context.Context, u entities.User) (*entities.User, error) {
	var row userRow
	err := a.db.GetContext(ctx, &row, `INSERT INTO users(email,username,display_name,password_hash,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6) RETURNING `+userColumns, u.Email(), u.Username(), u.DisplayName(), u.PasswordHash(), u.CreatedAt(), u.UpdatedAt())
	var pg *pq.Error
	if errors.As(err, &pg) && pg.Code == "23505" {
		return nil, usecase.ErrAccountExists
	}
	if err != nil {
		return nil, err
	}
	return row.user()
}
func (a *Auth) FindLogin(ctx context.Context, key string) (*entities.User, error) {
	var rows []userRow
	if err := a.db.SelectContext(ctx, &rows, `SELECT `+userColumns+` FROM users WHERE lower(btrim(email))=$1 OR lower(username)=$1 LIMIT 2`, key); err != nil {
		return nil, err
	}
	if len(rows) != 1 {
		return nil, nil
	}
	return rows[0].user()
}
func (a *Auth) Rehash(ctx context.Context, id int64, old, next string) error {
	_, err := a.db.ExecContext(ctx, `UPDATE users SET password_hash=$3 WHERE id=$1 AND password_hash=$2`, id, old, next)
	return err
}
func (a *Auth) CreateSession(ctx context.Context, s entities.AuthSession, hash []byte) error {
	tx, err := a.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO auth_sessions(id,user_id,created_at,expires_at,last_used_at) VALUES($1,$2,$3,$4,$5)`, s.ID, s.UserID, s.CreatedAt, s.ExpiresAt, s.LastUsedAt); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO auth_refresh_tokens(token_hash,session_id) VALUES($1,$2)`, hash, s.ID); err != nil {
		return err
	}
	return tx.Commit()
}
func scanSession(row *sql.Row) (*entities.AuthSession, error) {
	var s entities.AuthSession
	err := row.Scan(&s.ID, &s.UserID, &s.CreatedAt, &s.ExpiresAt, &s.LastUsedAt, &s.RevokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &s, err
}

const sessionColumns = `s.id,s.user_id,s.created_at,s.expires_at,s.last_used_at,s.revoked_at`

func (a *Auth) Session(ctx context.Context, hash []byte) (*entities.AuthSession, error) {
	var s entities.AuthSession
	err := a.db.QueryRowContext(ctx, `SELECT `+sessionColumns+`,t.used_at FROM auth_sessions s JOIN auth_refresh_tokens t ON t.session_id=s.id WHERE t.token_hash=$1`, hash).Scan(&s.ID, &s.UserID, &s.CreatedAt, &s.ExpiresAt, &s.LastUsedAt, &s.RevokedAt, &s.RefreshUsedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &s, err
}
func (a *Auth) Rotate(ctx context.Context, old, next []byte, sid string, logout bool) (*entities.AuthSession, error) {
	tx, err := a.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	s, err := scanSession(tx.QueryRowContext(ctx, `SELECT `+sessionColumns+` FROM auth_sessions s WHERE s.id=$1 FOR UPDATE`, sid))
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, usecase.ErrAuth
	}
	var used *time.Time
	err = tx.GetContext(ctx, &used, `SELECT used_at FROM auth_refresh_tokens WHERE token_hash=$1 AND session_id=$2`, old, sid)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if logout {
		if _, err = tx.ExecContext(ctx, `UPDATE auth_sessions SET revoked_at=COALESCE(revoked_at,$2) WHERE id=$1`, sid, now); err != nil {
			return nil, err
		}
		return s, tx.Commit()
	}
	if !s.Active(now) {
		return nil, usecase.ErrAuth
	}
	if used != nil {
		if now.Sub(*used) <= 5*time.Second {
			return nil, usecase.ErrRefreshRace
		}
		if _, err = tx.ExecContext(ctx, `UPDATE auth_sessions SET revoked_at=$2 WHERE id=$1`, sid, now); err != nil {
			return nil, err
		}
		if err = tx.Commit(); err != nil {
			return nil, err
		}
		return nil, usecase.ErrAuth
	}
	if _, err = tx.ExecContext(ctx, `UPDATE auth_refresh_tokens SET used_at=$2 WHERE token_hash=$1`, old, now); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO auth_refresh_tokens(token_hash,session_id) VALUES($1,$2)`, next, sid); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE auth_sessions SET last_used_at=$2 WHERE id=$1`, sid, now); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return s, nil
}
