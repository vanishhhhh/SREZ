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

// Database time is authoritative for token expiry and persistent rate windows.
func emailNow(ctx context.Context, tx *sqlx.Tx) (time.Time, error) {
	var now time.Time
	err := tx.GetContext(ctx, &now, `SELECT clock_timestamp()`)
	return now, err
}
func reserveEmailIP(ctx context.Context, tx *sqlx.Tx, hash []byte, p usecase.VerificationPolicy, now time.Time) error {
	return reserveChallengeIP(ctx, tx, hash, p, now, "email_verification_ip_limits")
}
func reserveChallengeIP(ctx context.Context, tx *sqlx.Tx, hash []byte, p usecase.VerificationPolicy, now time.Time, table string) error {
	// Only server-owned identifiers are accepted here.
	if table != "email_verification_ip_limits" && table != "password_reset_ip_limits" {
		return usecase.ErrVerificationUnavailable
	}
	if p.IPMinute < 1 || p.IPHour < 1 || p.AccountHour < 1 {
		return usecase.ErrVerificationUnavailable
	}
	if _, e := tx.ExecContext(ctx, `INSERT INTO `+table+` VALUES($1,$2,$2,0,0) ON CONFLICT DO NOTHING`, hash, now); e != nil {
		return e
	}
	var r struct {
		Minute, Hour time.Time
		M, H         int
	}
	if e := tx.QueryRowContext(ctx, `SELECT minute_started_at,hour_started_at,minute_count,hour_count FROM `+table+` WHERE ip_hash=$1 FOR UPDATE`, hash).Scan(&r.Minute, &r.Hour, &r.M, &r.H); e != nil {
		return e
	}
	if !now.Before(r.Minute.Add(time.Minute)) {
		r.Minute = now
		r.M = 0
	}
	if !now.Before(r.Hour.Add(time.Hour)) {
		r.Hour = now
		r.H = 0
	}
	if r.M >= p.IPMinute || r.H >= p.IPHour {
		return usecase.ErrVerificationRate
	}
	_, e := tx.ExecContext(ctx, `UPDATE `+table+` SET minute_started_at=$2,hour_started_at=$3,minute_count=$4,hour_count=$5 WHERE ip_hash=$1`, hash, r.Minute, r.Hour, r.M+1, r.H+1)
	return e
}
func (a *Auth) RegisterUnverified(ctx context.Context, u entities.User, hash, ip []byte, p usecase.VerificationPolicy) (*entities.User, error) {
	tx, e := a.db.BeginTxx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	now, e := emailNow(ctx, tx)
	if e != nil {
		return nil, e
	}
	if e = reserveEmailIP(ctx, tx, ip, p, now); e != nil {
		return nil, e
	}
	var row userRow
	e = tx.GetContext(ctx, &row, `INSERT INTO users(email,username,display_name,password_hash,created_at,updated_at,email_verification_required) VALUES($1,$2,$3,$4,$5,$5,true) RETURNING `+userColumns, u.Email(), u.Username(), u.DisplayName(), u.PasswordHash(), now)
	var pg *pq.Error
	if errors.As(e, &pg) && pg.Code == "23505" {
		return nil, usecase.ErrAccountExists
	}
	if e != nil {
		return nil, e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO email_verification_tokens(user_id,token_hash,created_at,expires_at) VALUES($1,$2,$3::timestamptz,$3::timestamptz+interval '30 minutes')`, row.ID, hash, now); e != nil {
		return nil, e
	}
	saved, e := row.user()
	if e != nil {
		return nil, e
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	return saved, nil
}
func (a *Auth) RotateVerification(ctx context.Context, identifier string, hash, ip []byte, p usecase.VerificationPolicy) (*entities.User, error) {
	tx, e := a.db.BeginTxx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	now, e := emailNow(ctx, tx)
	if e != nil {
		return nil, e
	}
	if e = reserveEmailIP(ctx, tx, ip, p, now); e != nil {
		return nil, e
	}
	var rows []userRow
	if e = tx.SelectContext(ctx, &rows, `SELECT `+userColumns+` FROM users WHERE lower(btrim(email))=$1 OR lower(username)=$1 ORDER BY id LIMIT 2 FOR UPDATE`, identifier); e != nil {
		return nil, e
	}
	if len(rows) != 1 || rows[0].EmailVerifiedAt != nil {
		return nil, tx.Commit()
	}
	row := rows[0]
	var count int
	var last *time.Time
	if e = tx.QueryRowContext(ctx, `SELECT count(*) FILTER(WHERE created_at>$2::timestamptz-interval '1 hour'),max(created_at) FROM email_verification_tokens WHERE user_id=$1`, row.ID, now).Scan(&count, &last); e != nil {
		return nil, e
	}
	// Account-specific suppression is deliberately indistinguishable from an
	// unknown/already verified account. IP quotas still consume and commit.
	if count >= p.AccountHour || (last != nil && now.Before(last.Add(time.Minute))) {
		return nil, tx.Commit()
	}
	if _, e = tx.ExecContext(ctx, `UPDATE email_verification_tokens SET revoked_at=GREATEST(created_at,$2) WHERE user_id=$1 AND used_at IS NULL AND revoked_at IS NULL`, row.ID, now); e != nil {
		return nil, e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO email_verification_tokens(user_id,token_hash,created_at,expires_at) VALUES($1,$2,$3::timestamptz,$3::timestamptz+interval '30 minutes')`, row.ID, hash, now); e != nil {
		return nil, e
	}
	user, e := row.user()
	if e != nil {
		return nil, e
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	return user, nil
}
func (a *Auth) VerifyEmail(ctx context.Context, hash []byte) error {
	tx, e := a.db.BeginTxx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var uid int64
	if e = tx.GetContext(ctx, &uid, `SELECT user_id FROM email_verification_tokens WHERE token_hash=$1`, hash); errors.Is(e, sql.ErrNoRows) {
		return usecase.ErrVerificationToken
	} else if e != nil {
		return e
	}
	var verified *time.Time
	if e = tx.GetContext(ctx, &verified, `SELECT email_verified_at FROM users WHERE id=$1 FOR UPDATE`, uid); errors.Is(e, sql.ErrNoRows) {
		return usecase.ErrVerificationToken
	} else if e != nil {
		return e
	}
	if verified != nil {
		return usecase.ErrVerificationToken
	}
	now, e := emailNow(ctx, tx)
	if e != nil {
		return e
	}
	r, e := tx.ExecContext(ctx, `UPDATE email_verification_tokens SET used_at=$2 WHERE token_hash=$1 AND used_at IS NULL AND revoked_at IS NULL AND expires_at>$2 AND created_at<=$2`, hash, now)
	if e != nil {
		return e
	}
	n, e := r.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return usecase.ErrVerificationToken
	}
	if _, e = tx.ExecContext(ctx, `UPDATE users SET email_verified_at=$2,updated_at=GREATEST(updated_at,$2) WHERE id=$1`, uid, now); e != nil {
		return e
	}
	return tx.Commit()
}
