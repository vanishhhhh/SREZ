package storage

import (
	"context"
	"database/sql"
	"errors"
	"srez/internal/entities"
	"srez/internal/usecase"
	"time"
)

func (a *Auth) IssuePasswordReset(ctx context.Context, email string, hash, ip []byte, p usecase.VerificationPolicy) (*entities.User, error) {
	tx, err := a.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	now, err := emailNow(ctx, tx)
	if err != nil {
		return nil, err
	}
	if err = reserveChallengeIP(ctx, tx, ip, p, now, "password_reset_ip_limits"); err != nil {
		if errors.Is(err, usecase.ErrVerificationRate) {
			return nil, usecase.ErrRecoveryRate
		}
		return nil, err
	}
	var row userRow
	err = tx.GetContext(ctx, &row, `SELECT `+userColumns+` FROM users WHERE lower(btrim(email))=$1 FOR UPDATE`, email)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, tx.Commit()
	}
	if err != nil {
		return nil, err
	}
	// A legacy user can verify voluntarily before enabling email recovery.
	if row.EmailVerifiedAt == nil || row.PasswordHash == "!" {
		return nil, tx.Commit()
	}
	var count int
	var last *time.Time
	err = tx.QueryRowContext(ctx, `SELECT count(*) FILTER(WHERE created_at>$2::timestamptz-interval '1 hour'),max(created_at) FROM password_reset_tokens WHERE user_id=$1`, row.ID, now).Scan(&count, &last)
	if err != nil {
		return nil, err
	}
	if count >= p.AccountHour || (last != nil && now.Before(last.Add(time.Minute))) {
		return nil, tx.Commit()
	}
	if _, err = tx.ExecContext(ctx, `UPDATE password_reset_tokens SET revoked_at=GREATEST(created_at,$2) WHERE user_id=$1 AND used_at IS NULL AND revoked_at IS NULL`, row.ID, now); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO password_reset_tokens(user_id,token_hash,created_at,expires_at) VALUES($1,$2,$3::timestamptz,$3::timestamptz+interval '30 minutes')`, row.ID, hash, now); err != nil {
		return nil, err
	}
	u, err := row.user()
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return u, nil
}
func (a *Auth) ResetPassword(ctx context.Context, hash []byte, passwordHash string) error {
	tx, err := a.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var uid int64
	err = tx.GetContext(ctx, &uid, `SELECT user_id FROM password_reset_tokens WHERE token_hash=$1`, hash)
	if errors.Is(err, sql.ErrNoRows) {
		return usecase.ErrResetToken
	}
	if err != nil {
		return err
	}
	// Global lock order: user -> token -> sessions, including login/refresh.
	var verified *time.Time
	err = tx.GetContext(ctx, &verified, `SELECT email_verified_at FROM users WHERE id=$1 FOR UPDATE`, uid)
	if errors.Is(err, sql.ErrNoRows) {
		return usecase.ErrResetToken
	}
	if err != nil {
		return err
	}
	if verified == nil {
		return usecase.ErrResetToken
	}
	now, err := emailNow(ctx, tx)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE password_reset_tokens SET used_at=$2 WHERE token_hash=$1 AND used_at IS NULL AND revoked_at IS NULL AND expires_at>$2 AND created_at<=$2`, hash, now)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return usecase.ErrResetToken
	}
	if _, err = tx.ExecContext(ctx, `UPDATE users SET password_hash=$2,updated_at=GREATEST(updated_at,$3) WHERE id=$1`, uid, passwordHash, now); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE auth_sessions SET revoked_at=GREATEST(created_at,$2) WHERE user_id=$1 AND revoked_at IS NULL`, uid, now); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE password_reset_tokens SET revoked_at=GREATEST(created_at,$2) WHERE user_id=$1 AND used_at IS NULL AND revoked_at IS NULL`, uid, now); err != nil {
		return err
	}
	return tx.Commit()
}
