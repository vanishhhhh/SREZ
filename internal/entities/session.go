package entities

import "time"

type AuthSession struct {
	ID                               string
	UserID                           int64
	CreatedAt, ExpiresAt, LastUsedAt time.Time
	RefreshUsedAt                    *time.Time
	RevokedAt                        *time.Time
}

func (s AuthSession) Active(now time.Time) bool {
	return s.ID != "" && s.UserID > 0 && s.RevokedAt == nil && now.Before(s.ExpiresAt) && !s.CreatedAt.After(now) && !s.ExpiresAt.After(s.CreatedAt.Add(7*24*time.Hour))
}
