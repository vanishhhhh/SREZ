package storage

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"srez/internal/entities"
	"srez/internal/usecase"
)

type Management struct{ db *sqlx.DB }

func NewManagement(db *sqlx.DB) *Management { return &Management{db} }
func managementError(e error) error {
	var pg *pq.Error
	if errors.As(e, &pg) && pg.Code == "23505" {
		return usecase.ErrConflict
	}
	return e
}
func (s *Management) WithinBrand(ctx context.Context, id int64, fn func(usecase.ManagementTx, *entities.Brand) error) error {
	tx, e := s.db.BeginTxx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var row brandRow
	e = tx.GetContext(ctx, &row, `SELECT `+brandColumns+` FROM brands WHERE id=$1 FOR UPDATE`, id)
	if errors.Is(e, sql.ErrNoRows) {
		return usecase.ErrResourceHidden
	}
	if e != nil {
		return e
	}
	b, e := row.brand()
	if e != nil {
		return e
	}
	if e = fn(&managementTx{tx, id}, b); e != nil {
		return managementError(e)
	}
	return managementError(tx.Commit())
}
func (s *Management) CreateBrand(ctx context.Context, uid int64, b entities.Brand) (*entities.Brand, error) {
	tx, e := s.db.BeginTxx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	var row brandRow
	e = tx.GetContext(ctx, &row, `INSERT INTO brands(name,slug,status,created_at,updated_at) VALUES($1,$2,'draft',$3,$4) RETURNING `+brandColumns, b.Name(), b.Slug(), b.CreatedAt(), b.UpdatedAt())
	if e != nil {
		return nil, managementError(e)
	}
	saved, e := row.brand()
	if e != nil {
		return nil, e
	}
	owner, e := entities.NewOwnerMembership(uid, saved.ID(), time.Now().UTC())
	if e != nil {
		return nil, e
	}
	adapter := managementTx{tx, saved.ID()}
	if e = adapter.SaveMembership(ctx, *owner); e != nil {
		return nil, managementError(e)
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	return saved, nil
}
func (s *Management) ProductBrand(ctx context.Context, id int64) (int64, error) {
	var brand int64
	e := s.db.GetContext(ctx, &brand, `SELECT brand_id FROM products WHERE id=$1`, id)
	if errors.Is(e, sql.ErrNoRows) {
		return 0, usecase.ErrResourceHidden
	}
	return brand, e
}
func (s *Management) MyBrands(ctx context.Context, uid int64, invitations bool) ([]usecase.ManagedBrand, error) {
	status := "active"
	if invitations {
		status = "invited"
	}
	columns := "b." + strings.ReplaceAll(brandColumns, ", ", ", b.")
	var rows []struct {
		brandRow
		Role entities.BrandRole `db:"membership_role"`
	}
	e := s.db.SelectContext(ctx, &rows, `SELECT `+columns+`,m.role AS membership_role FROM brands b JOIN brand_memberships m ON m.brand_id=b.id WHERE m.user_id=$1 AND m.status=$2 ORDER BY b.id`, uid, status)
	if e != nil {
		return nil, e
	}
	result := make([]usecase.ManagedBrand, 0, len(rows))
	for _, r := range rows {
		b, e := r.brand()
		if e != nil {
			return nil, e
		}
		result = append(result, usecase.ManagedBrand{Brand: *b, Role: r.Role})
	}
	return result, nil
}

type managementTx struct {
	tx      *sqlx.Tx
	brandID int64
}
type membershipRow struct {
	UserID    int64                     `db:"user_id"`
	BrandID   int64                     `db:"brand_id"`
	Role      entities.BrandRole        `db:"role"`
	Status    entities.MembershipStatus `db:"status"`
	InvitedBy *int64                    `db:"invited_by_user_id"`
	InvitedAt *time.Time                `db:"invited_at"`
	JoinedAt  *time.Time                `db:"joined_at"`
	CreatedAt time.Time                 `db:"created_at"`
	UpdatedAt time.Time                 `db:"updated_at"`
}

func (r membershipRow) membership() (*entities.BrandMembership, error) {
	return entities.RestoreBrandMembership(entities.BrandMembershipState{UserID: r.UserID, BrandID: r.BrandID, Role: r.Role, Status: r.Status, InvitedByUserID: r.InvitedBy, InvitedAt: r.InvitedAt, JoinedAt: r.JoinedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt})
}

const membershipColumns = `user_id,brand_id,role,status,invited_by_user_id,invited_at,joined_at,created_at,updated_at`

func (t *managementTx) Membership(ctx context.Context, uid int64) (*entities.BrandMembership, error) {
	var row membershipRow
	e := t.tx.GetContext(ctx, &row, `SELECT `+membershipColumns+` FROM brand_memberships WHERE brand_id=$1 AND user_id=$2 FOR UPDATE`, t.brandID, uid)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	return row.membership()
}
func (t *managementTx) SaveMembership(ctx context.Context, m entities.BrandMembership) error {
	if m.BrandID() != t.brandID {
		return usecase.ErrResourceHidden
	}
	s := m.State()
	_, e := t.tx.ExecContext(ctx, `INSERT INTO brand_memberships(user_id,brand_id,role,status,invited_by_user_id,invited_at,joined_at,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(user_id,brand_id) DO UPDATE SET status=excluded.status,invited_by_user_id=excluded.invited_by_user_id,invited_at=excluded.invited_at,joined_at=excluded.joined_at,updated_at=excluded.updated_at`, s.UserID, s.BrandID, s.Role, s.Status, s.InvitedByUserID, s.InvitedAt, s.JoinedAt, s.CreatedAt, s.UpdatedAt)
	return e
}
func (t *managementTx) FindUser(ctx context.Context, username string) (int64, error) {
	var uid int64
	e := t.tx.GetContext(ctx, &uid, `SELECT id FROM users WHERE lower(username)=$1`, strings.ToLower(strings.TrimSpace(username)))
	if errors.Is(e, sql.ErrNoRows) {
		return 0, nil
	}
	return uid, e
}
func (t *managementTx) Members(ctx context.Context) ([]entities.BrandMembership, error) {
	var rows []membershipRow
	e := t.tx.SelectContext(ctx, &rows, `SELECT `+membershipColumns+` FROM brand_memberships WHERE brand_id=$1 ORDER BY user_id`, t.brandID)
	if e != nil {
		return nil, e
	}
	result := make([]entities.BrandMembership, 0, len(rows))
	for _, r := range rows {
		m, e := r.membership()
		if e != nil {
			return nil, e
		}
		result = append(result, *m)
	}
	return result, nil
}
func (t *managementTx) SaveBrand(ctx context.Context, b entities.Brand) error {
	if b.ID() != t.brandID {
		return usecase.ErrResourceHidden
	}
	c, a := b.Contacts(), b.Appearance()
	_, e := t.tx.ExecContext(ctx, `UPDATE brands SET name=$2,slug=$3,description=$4,logo_url=$5,banner_url=$6,contact_email=$7,telegram_url=$8,website_url=$9,accent_color=$10,background_color=$11,text_color=$12,status=$13,updated_at=$14 WHERE id=$1`, b.ID(), b.Name(), b.Slug(), b.Description(), b.LogoURL(), b.BannerURL(), c.Email, c.TelegramURL, c.WebsiteURL, a.AccentColor, a.BackgroundColor, a.TextColor, b.Status(), b.UpdatedAt())
	return e
}
func (t *managementTx) Products(ctx context.Context) ([]entities.Product, error) {
	var rows []productRow
	e := t.tx.SelectContext(ctx, &rows, `SELECT `+productColumns+` FROM products p WHERE brand_id=$1 ORDER BY id`, t.brandID)
	if e != nil {
		return nil, e
	}
	result := make([]entities.Product, 0, len(rows))
	for _, r := range rows {
		p, e := r.product()
		if e != nil {
			return nil, e
		}
		result = append(result, *p)
	}
	return result, nil
}
func (t *managementTx) Product(ctx context.Context, id int64) (*entities.Product, error) {
	var r productRow
	e := t.tx.GetContext(ctx, &r, `SELECT `+productColumns+` FROM products p WHERE id=$1 AND brand_id=$2 FOR UPDATE`, id, t.brandID)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	return r.product()
}
func (t *managementTx) Publication(ctx context.Context, id int64) (entities.ProductPublication, error) {
	return loadPublication(ctx, t.tx, id, true)
}
func (t *managementTx) SaveProduct(ctx context.Context, p entities.Product) error {
	if p.BrandID() != t.brandID {
		return usecase.ErrResourceHidden
	}
	materials := p.Materials()
	if materials == nil {
		materials = []string{}
	}
	_, e := t.tx.ExecContext(ctx, `UPDATE products SET name=$3,description=$4,price_kopecks=$5,category=$6,materials=$7,status=$8,updated_at=$9 WHERE id=$1 AND brand_id=$2`, p.ID(), p.BrandID(), p.Name(), p.Description(), p.PriceKopecks(), p.Category(), pq.Array(materials), p.Status(), p.UpdatedAt())
	return e
}
func (t *managementTx) InsertProduct(ctx context.Context, p entities.Product) (*entities.Product, error) {
	if p.BrandID() != t.brandID {
		return nil, usecase.ErrResourceHidden
	}
	materials := p.Materials()
	if materials == nil {
		materials = []string{}
	}
	var id int64
	e := t.tx.GetContext(ctx, &id, `INSERT INTO products(brand_id,name,description,price_kopecks,category,materials,status,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,'draft',$7,$8) RETURNING id`, p.BrandID(), p.Name(), p.Description(), p.PriceKopecks(), p.Category(), pq.Array(materials), p.CreatedAt(), p.UpdatedAt())
	if e != nil {
		return nil, e
	}
	state := p.State()
	state.ID = id
	return entities.RestoreProduct(state)
}
