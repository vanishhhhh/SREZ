package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	"srez/internal/entities"
	"srez/internal/usecase"
)

type Brands struct{ db *sqlx.DB }

func NewBrands(db *sqlx.DB) *Brands { return &Brands{db: db} }

var _ usecase.BrandRepository = (*Brands)(nil)

type brandRow struct {
	ID              int64                `db:"id"`
	Name            string               `db:"name"`
	Slug            string               `db:"slug"`
	Description     string               `db:"description"`
	LogoURL         string               `db:"logo_url"`
	BannerURL       string               `db:"banner_url"`
	Email           string               `db:"contact_email"`
	TelegramURL     string               `db:"telegram_url"`
	WebsiteURL      string               `db:"website_url"`
	AccentColor     string               `db:"accent_color"`
	BackgroundColor string               `db:"background_color"`
	TextColor       string               `db:"text_color"`
	Status          entities.BrandStatus `db:"status"`
	CreatedAt       time.Time            `db:"created_at"`
	UpdatedAt       time.Time            `db:"updated_at"`
}

const brandColumns = `id, name, slug, description, logo_url, banner_url, contact_email, telegram_url, website_url, accent_color, background_color, text_color, status, created_at, updated_at`

func (r brandRow) brand() (*entities.Brand, error) {
	return entities.RestoreBrand(entities.BrandState{ID: r.ID, Name: r.Name, Slug: r.Slug, Description: r.Description, LogoURL: r.LogoURL, BannerURL: r.BannerURL,
		Contacts:   entities.BrandContacts{Email: r.Email, TelegramURL: r.TelegramURL, WebsiteURL: r.WebsiteURL},
		Appearance: entities.BrandAppearance{AccentColor: r.AccentColor, BackgroundColor: r.BackgroundColor, TextColor: r.TextColor}, Status: r.Status, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt})
}
func (b *Brands) ListActive(ctx context.Context) ([]entities.Brand, error) {
	var rows []brandRow
	if err := b.db.SelectContext(ctx, &rows, `SELECT `+brandColumns+` FROM brands WHERE status = 'active' ORDER BY id`); err != nil {
		return nil, err
	}
	result := make([]entities.Brand, 0, len(rows))
	for _, r := range rows {
		brand, err := r.brand()
		if err != nil {
			return nil, err
		}
		result = append(result, *brand)
	}
	return result, nil
}
func (b *Brands) get(ctx context.Context, query string, arg any) (*entities.Brand, error) {
	var row brandRow
	err := b.db.GetContext(ctx, &row, query, arg)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return row.brand()
}
func (b *Brands) GetActiveBySlug(ctx context.Context, slug string) (*entities.Brand, error) {
	return b.get(ctx, `SELECT `+brandColumns+` FROM brands WHERE slug = $1 AND status = 'active'`, slug)
}
func (b *Brands) Get(ctx context.Context, id int64) (*entities.Brand, error) {
	return b.get(ctx, `SELECT `+brandColumns+` FROM brands WHERE id = $1`, id)
}
