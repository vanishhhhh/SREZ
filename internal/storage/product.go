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

type Products struct{ db *sqlx.DB }

func NewProducts(db *sqlx.DB) *Products { return &Products{db: db} }

var _ usecase.ProductRepository = (*Products)(nil)

type productRow struct {
	ID           int64                    `db:"id"`
	BrandID      int64                    `db:"brand_id"`
	Name         string                   `db:"name"`
	Description  string                   `db:"description"`
	PriceKopecks int64                    `db:"price_kopecks"`
	Category     entities.ProductCategory `db:"category"`
	Materials    pq.StringArray           `db:"materials"`
	Status       entities.ProductStatus   `db:"status"`
	CreatedAt    time.Time                `db:"created_at"`
	UpdatedAt    time.Time                `db:"updated_at"`
}

const productColumns = `p.id, p.brand_id, p.name, p.description, p.price_kopecks, p.category, p.materials, p.status, p.created_at, p.updated_at`
const publicProducts = ` FROM products p JOIN brands b ON b.id = p.brand_id WHERE p.status = 'published' AND b.status = 'active'`

func (r productRow) product() (*entities.Product, error) {
	return entities.RestoreProduct(entities.ProductState{ID: r.ID, BrandID: r.BrandID, Name: r.Name, Description: r.Description, PriceKopecks: r.PriceKopecks, Category: r.Category, Materials: []string(r.Materials), Status: r.Status, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt})
}
func (p *Products) ListPublic(ctx context.Context) ([]entities.Product, error) {
	var rows []productRow
	if err := p.db.SelectContext(ctx, &rows, `SELECT `+productColumns+publicProducts+` ORDER BY p.id`); err != nil {
		return nil, err
	}
	result := make([]entities.Product, 0, len(rows))
	for _, r := range rows {
		product, err := r.product()
		if err != nil {
			return nil, err
		}
		result = append(result, *product)
	}
	return result, nil
}

// GetPublic reads the complete aggregate from one consistent snapshot. Images
// and variants are fetched in batches, without a Cartesian join or N+1 queries.
func (p *Products) GetPublic(ctx context.Context, id int64) (*usecase.ProductDetail, error) {
	tx, err := p.db.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var row productRow
	err = tx.GetContext(ctx, &row, `SELECT `+productColumns+publicProducts+` AND p.id = $1`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	product, err := row.product()
	if err != nil {
		return nil, err
	}
	publication, err := loadPublication(ctx, tx, id, false)
	if err != nil {
		return nil, err
	}
	if err = product.ValidatePublication(publication); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &usecase.ProductDetail{Product: *product, Publication: publication}, nil
}
func (p *Products) InsertDraft(ctx context.Context, product entities.Product) (*entities.Product, error) {
	if product.Status() != entities.ProductDraft || product.ID() != 0 {
		return nil, entities.ErrInvalidValue
	}
	tx, err := p.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	// FOR SHARE blocks deletion and status changes until the insert commits.
	var status entities.BrandStatus
	err = tx.GetContext(ctx, &status, `SELECT status FROM brands WHERE id=$1 FOR SHARE`, product.BrandID())
	if errors.Is(err, sql.ErrNoRows) {
		return nil, usecase.ErrBrandNotFound
	}
	if err != nil {
		return nil, err
	}
	if status != entities.BrandActive {
		return nil, usecase.ErrBrandInactive
	}
	materials := product.Materials()
	if materials == nil {
		materials = []string{}
	}
	var id int64
	err = tx.GetContext(ctx, &id, `INSERT INTO products(brand_id,name,description,price_kopecks,category,materials,status,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,'draft',$7,$8) RETURNING id`, product.BrandID(), product.Name(), product.Description(), product.PriceKopecks(), product.Category(), pq.Array(materials), product.CreatedAt(), product.UpdatedAt())
	if err != nil {
		return nil, err
	}
	state := product.State()
	state.ID = id
	saved, err := entities.RestoreProduct(state)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return saved, nil
}

func loadPublication(ctx context.Context, tx *sqlx.Tx, id int64, lock bool) (entities.ProductPublication, error) {
	colorLock, imageLock, variantLock := "", "", ""
	if lock {
		colorLock = " FOR UPDATE"
		imageLock = " FOR UPDATE OF i"
		variantLock = " FOR UPDATE OF v"
	}
	var err error
	var colors []struct {
		ID    int64  `db:"id"`
		Color string `db:"color"`
	}
	if err = tx.SelectContext(ctx, &colors, `SELECT id,color FROM product_colors WHERE product_id = $1 ORDER BY id`+colorLock, id); err != nil {
		return entities.ProductPublication{}, err
	}
	var images []struct {
		ColorID int64  `db:"product_color_id"`
		URL     string `db:"url"`
	}
	if err = tx.SelectContext(ctx, &images, `SELECT i.product_color_id,i.url FROM product_color_images i JOIN product_colors c ON c.id=i.product_color_id WHERE c.product_id=$1 ORDER BY i.product_color_id,i.position`+imageLock, id); err != nil {
		return entities.ProductPublication{}, err
	}
	var variants []struct {
		ID      int64  `db:"id"`
		ColorID int64  `db:"product_color_id"`
		Size    string `db:"size"`
		Stock   int    `db:"stock"`
		SKU     string `db:"sku"`
	}
	if err = tx.SelectContext(ctx, &variants, `SELECT v.id,v.product_color_id,v.size,v.stock,v.sku FROM product_variants v JOIN product_colors c ON c.id=v.product_color_id WHERE c.product_id=$1 ORDER BY v.id`+variantLock, id); err != nil {
		return entities.ProductPublication{}, err
	}
	publication := entities.ProductPublication{Colors: make([]entities.ProductColor, 0, len(colors)), Variants: make([]entities.ProductVariant, 0, len(variants))}
	imageMap := map[int64][]string{}
	for _, image := range images {
		imageMap[image.ColorID] = append(imageMap[image.ColorID], image.URL)
	}
	for _, c := range colors {
		urls := imageMap[c.ID]
		if urls == nil {
			urls = []string{}
		}
		color, err := entities.RestoreProductColor(entities.ProductColorState{ID: c.ID, ProductID: id, Color: c.Color, Images: urls})
		if err != nil {
			return entities.ProductPublication{}, err
		}
		publication.Colors = append(publication.Colors, *color)
	}
	for _, v := range variants {
		variant, err := entities.RestoreProductVariant(entities.ProductVariantState{ID: v.ID, ProductColorID: v.ColorID, Size: v.Size, Stock: v.Stock, SKU: v.SKU})
		if err != nil {
			return entities.ProductPublication{}, err
		}
		publication.Variants = append(publication.Variants, *variant)
	}
	return publication, nil
}
