package handler

import (
	"context"

	"srez/internal/entities"
	"srez/internal/usecase"
)

type catalogRepository struct {
	brands   []entities.Brand
	products []mockProductEntry
	nextID   int64
	err      error
}

func newCatalogRepository() *catalogRepository {
	brands, err := mockBrands()
	if err != nil {
		panic(err)
	}
	products, err := mockProductCatalog()
	if err != nil {
		panic(err)
	}
	return &catalogRepository{brands: brands, products: products, nextID: 3}
}
func newTestCatalogHandler() *Handler {
	r := newCatalogRepository()
	return New(usecase.NewUsers(mockUserRepository{}), usecase.NewBrands(r), usecase.NewProducts(r, r), true)
}
func (r *catalogRepository) ListActive(context.Context) ([]entities.Brand, error) {
	if r.err != nil {
		return nil, r.err
	}
	result := []entities.Brand{}
	for _, b := range r.brands {
		if b.Status() == entities.BrandActive {
			result = append(result, b)
		}
	}
	return result, nil
}
func (r *catalogRepository) GetActiveBySlug(ctx context.Context, slug string) (*entities.Brand, error) {
	brands, err := r.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	for _, b := range brands {
		if b.Slug() == slug {
			return &b, nil
		}
	}
	return nil, nil
}
func (r *catalogRepository) Get(_ context.Context, id int64) (*entities.Brand, error) {
	if r.err != nil {
		return nil, r.err
	}
	for _, b := range r.brands {
		if b.ID() == id {
			return &b, nil
		}
	}
	return nil, nil
}
func (r *catalogRepository) ListPublic(ctx context.Context) ([]entities.Product, error) {
	if r.err != nil {
		return nil, r.err
	}
	result := []entities.Product{}
	for _, p := range r.products {
		b, _ := r.Get(ctx, p.product.BrandID())
		if p.product.Status() == entities.ProductPublished && b != nil && b.Status() == entities.BrandActive {
			result = append(result, p.product)
		}
	}
	return result, nil
}

// Existing HTTP fixtures represent a fixed two-item page. SQL selection is
// tested against PostgreSQL; parameter forwarding uses a separate spy.
func (r *catalogRepository) BrowsePublic(ctx context.Context, q usecase.ProductQuery) (*usecase.ProductPage, error) {
	products, err := r.ListPublic(ctx)
	if err != nil {
		return nil, err
	}
	page := &usecase.ProductPage{Items: []usecase.ProductCard{}, Total: int64(len(products)), Page: q.Page, Limit: q.Limit}
	for _, product := range products {
		brand, _ := r.Get(ctx, product.BrandID())
		detail, _ := r.GetPublic(ctx, product.ID())
		card := usecase.ProductCard{Product: product, BrandName: brand.Name(), BrandSlug: brand.Slug(), Colors: []usecase.ProductCardColor{}}
		for _, color := range detail.Publication.Colors {
			item := usecase.ProductCardColor{ID: color.ID(), Color: color.Color(), Variants: []usecase.ProductCardVariant{}}
			if images := color.Images(); len(images) > 0 {
				item.PreviewImageURL = images[0]
				if card.PreviewImageURL == "" {
					card.PreviewImageURL = images[0]
				}
			}
			for _, variant := range detail.Publication.Variants {
				if variant.ProductColorID() == color.ID() {
					item.Variants = append(item.Variants, usecase.ProductCardVariant{ID: variant.ID(), Size: variant.Size(), Stock: int64(variant.Stock())})
				}
			}
			card.Colors = append(card.Colors, item)
		}
		page.Items = append(page.Items, card)
	}
	return page, nil
}
func (r *catalogRepository) GetPublic(ctx context.Context, id int64) (*usecase.ProductDetail, error) {
	products, err := r.ListPublic(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range products {
		if p.ID() == id {
			for _, e := range r.products {
				if e.product.ID() == id {
					return &usecase.ProductDetail{Product: p, Publication: e.publication}, nil
				}
			}
		}
	}
	return nil, nil
}
func (r *catalogRepository) InsertDraft(_ context.Context, p entities.Product) (*entities.Product, error) {
	if r.err != nil {
		return nil, r.err
	}
	state := p.State()
	state.ID = r.nextID
	r.nextID++
	saved, err := entities.RestoreProduct(state)
	if err != nil {
		return nil, err
	}
	r.products = append(r.products, mockProductEntry{product: *saved})
	return saved, nil
}
