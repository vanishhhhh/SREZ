package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"srez/internal/entities"
)

type brandRepoStub struct {
	brand *entities.Brand
	err   error
	calls int
}

func (r *brandRepoStub) Get(context.Context, int64) (*entities.Brand, error) {
	r.calls++
	return r.brand, r.err
}
func (r *brandRepoStub) GetActiveBySlug(context.Context, string) (*entities.Brand, error) {
	return r.brand, r.err
}
func (r *brandRepoStub) ListActive(context.Context) ([]entities.Brand, error) { return nil, r.err }

type productRepoStub struct {
	inserted int
	err      error
	product  entities.Product
}

func (r *productRepoStub) ListPublic(context.Context) ([]entities.Product, error) { return nil, r.err }
func (r *productRepoStub) BrowsePublic(_ context.Context, q ProductQuery) (*ProductPage, error) {
	return &ProductPage{Items: []ProductCard{}, Page: q.Page, Limit: q.Limit}, r.err
}
func (r *productRepoStub) GetPublic(context.Context, int64) (*ProductDetail, error) {
	return nil, r.err
}
func (r *productRepoStub) InsertDraft(_ context.Context, p entities.Product) (*entities.Product, error) {
	r.inserted++
	r.product = p
	if r.err != nil {
		return nil, r.err
	}
	state := p.State()
	state.ID = 42
	return entities.RestoreProduct(state)
}
func TestCreateProductBrandRules(t *testing.T) {
	dbErr := errors.New("database failed")
	for _, tt := range []struct {
		name      string
		status    entities.BrandStatus
		missing   bool
		err, want error
	}{
		{name: "active", status: entities.BrandActive},
		{name: "draft", status: entities.BrandDraft, want: ErrBrandInactive},
		{name: "suspended", status: entities.BrandSuspended, want: ErrBrandInactive},
		{name: "missing", missing: true, want: ErrBrandNotFound},
		{name: "db failure", err: dbErr, want: dbErr},
	} {
		t.Run(tt.name, func(t *testing.T) {
			now := time.Now().UTC()
			var brand *entities.Brand
			if !tt.missing && tt.err == nil {
				var err error
				brand, err = entities.RestoreBrand(entities.BrandState{ID: 7, Name: "Brand", Slug: "brand", Description: "Description", LogoURL: "https://example.com/logo", Status: tt.status, CreatedAt: now, UpdatedAt: now})
				if err != nil {
					t.Fatal(err)
				}
			}
			brands := &brandRepoStub{brand: brand, err: tt.err}
			repo := &productRepoStub{}
			product, err := NewProducts(repo, brands).Create(context.Background(), 7, "  Товар  ", "", 0, "", nil)
			if !errors.Is(err, tt.want) {
				t.Fatalf("error %v, want %v", err, tt.want)
			}
			if tt.want != nil {
				if repo.inserted != 0 {
					t.Fatal("inserted for unavailable brand")
				}
				return
			}
			if product.ID() != 42 || product.Status() != entities.ProductDraft || product.Name() != "Товар" || repo.product.ID() != 0 || repo.product.BrandID() != 7 {
				t.Fatalf("unexpected saved draft: %+v", product.State())
			}
		})
	}
}
func TestCreateProductValidatesBeforeStorage(t *testing.T) {
	brands := &brandRepoStub{}
	repo := &productRepoStub{}
	products := NewProducts(repo, brands)
	for _, id := range []int64{0, -1, 1} {
		_, err := products.Create(context.Background(), id, "x", "", 0, "", nil)
		if !errors.Is(err, entities.ErrInvalidValue) {
			t.Fatal(err)
		}
	}
	if brands.calls != 0 || repo.inserted != 0 {
		t.Fatal("storage called for invalid request")
	}
}
func TestCatalogNotFoundAndStorageErrors(t *testing.T) {
	ctx := context.Background()
	brands := &brandRepoStub{}
	products := &productRepoStub{}
	if _, err := NewBrands(brands).Get(ctx, "missing"); !errors.Is(err, ErrBrandNotFound) {
		t.Fatal(err)
	}
	if _, err := NewProducts(products, brands).Get(ctx, 1); !errors.Is(err, ErrProductNotFound) {
		t.Fatal(err)
	}
	sentinel := errors.New("storage failure")
	brands.err = sentinel
	products.err = sentinel
	if _, err := NewBrands(brands).Get(ctx, "brand"); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	if _, err := NewProducts(products, brands).Get(ctx, 1); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
}
func TestCreatePropagatesAtomicBrandRecheck(t *testing.T) {
	now := time.Now().UTC()
	b, err := entities.RestoreBrand(entities.BrandState{ID: 1, Name: "Brand", Slug: "brand", Description: "Description", LogoURL: "https://example.com/logo", Status: entities.BrandActive, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []error{ErrBrandInactive, ErrBrandNotFound, errors.New("insert failure")} {
		repo := &productRepoStub{err: want}
		_, err := NewProducts(repo, &brandRepoStub{brand: b}).Create(context.Background(), 1, "Product", "", 0, "", nil)
		if !errors.Is(err, want) {
			t.Fatal(err)
		}
	}
}
