package usecase

import (
	"context"
	"errors"
	"time"

	"srez/internal/entities"
)

var (
	ErrBrandNotFound   = errors.New("brand not found")
	ErrBrandInactive   = errors.New("brand is not active")
	ErrProductNotFound = errors.New("product not found")
)

type BrandRepository interface {
	ListActive(context.Context) ([]entities.Brand, error)
	GetActiveBySlug(context.Context, string) (*entities.Brand, error)
	Get(context.Context, int64) (*entities.Brand, error)
}

type ProductDetail struct {
	Product     entities.Product
	Publication entities.ProductPublication
}

type ProductRepository interface {
	ListPublic(context.Context) ([]entities.Product, error)
	GetPublic(context.Context, int64) (*ProductDetail, error)
	// InsertDraft rechecks and locks the active brand while inserting.
	InsertDraft(context.Context, entities.Product) (*entities.Product, error)
}

type Brands struct{ repo BrandRepository }

func NewBrands(repo BrandRepository) *Brands                         { return &Brands{repo: repo} }
func (b *Brands) List(ctx context.Context) ([]entities.Brand, error) { return b.repo.ListActive(ctx) }
func (b *Brands) Get(ctx context.Context, slug string) (*entities.Brand, error) {
	brand, err := b.repo.GetActiveBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	if brand == nil {
		return nil, ErrBrandNotFound
	}
	return brand, nil
}

type Products struct {
	repo   ProductRepository
	brands BrandRepository
}

func NewProducts(repo ProductRepository, brands BrandRepository) *Products {
	return &Products{repo: repo, brands: brands}
}
func (p *Products) List(ctx context.Context) ([]entities.Product, error) {
	return p.repo.ListPublic(ctx)
}
func (p *Products) Get(ctx context.Context, id int64) (*ProductDetail, error) {
	detail, err := p.repo.GetPublic(ctx, id)
	if err != nil {
		return nil, err
	}
	if detail == nil {
		return nil, ErrProductNotFound
	}
	return detail, nil
}
func (p *Products) Create(ctx context.Context, brandID int64, name, description string, price int64, category entities.ProductCategory, materials []string) (*entities.Product, error) {
	product, err := entities.NewProduct(brandID, name, description, price, category, materials, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	brand, err := p.brands.Get(ctx, brandID)
	if err != nil {
		return nil, err
	}
	if brand == nil {
		return nil, ErrBrandNotFound
	}
	if brand.Status() != entities.BrandActive {
		return nil, ErrBrandInactive
	}
	return p.repo.InsertDraft(ctx, *product)
}
