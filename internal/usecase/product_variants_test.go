package usecase

import (
	"context"
	"errors"
	"math"
	"slices"
	"testing"
	"time"

	"srez/internal/entities"
)

type variantUnitTx struct {
	ManagementTx
	member  *entities.BrandMembership
	product *entities.Product
	data    entities.ProductPublication
	writes  int
}

func (t *variantUnitTx) Membership(context.Context, int64) (*entities.BrandMembership, error) {
	return t.member, nil
}
func (t *variantUnitTx) Product(context.Context, int64) (*entities.Product, error) {
	p := *t.product
	return &p, nil
}
func (t *variantUnitTx) Publication(context.Context, int64) (entities.ProductPublication, error) {
	return t.data, nil
}
func (t *variantUnitTx) SaveProduct(_ context.Context, p entities.Product) error {
	t.product = &p
	t.writes++
	return nil
}
func (t *variantUnitTx) InsertColor(_ context.Context, c entities.ProductColor) (*entities.ProductColor, error) {
	s := c.State()
	s.ID = int64(len(t.data.Colors) + 1)
	saved, err := entities.RestoreProductColor(s)
	if err == nil {
		t.data.Colors = append(t.data.Colors, *saved)
		t.writes++
	}
	return saved, err
}

type variantUnitRepo struct {
	ManagementRepository
	tx    *variantUnitTx
	brand *entities.Brand
}

func (r variantUnitRepo) ProductBrand(context.Context, int64) (int64, error) {
	return r.brand.ID(), nil
}
func (r variantUnitRepo) WithinBrand(ctx context.Context, _ int64, fn func(ManagementTx, *entities.Brand) error) error {
	before := *r.tx
	before.data.Colors = slices.Clone(r.tx.data.Colors)
	err := fn(r.tx, r.brand)
	if err != nil {
		*r.tx = before
	}
	return err
}
func variantUnitManagement(t *testing.T) (*Management, *variantUnitTx) {
	t.Helper()
	now := time.Now().UTC()
	b, err := entities.RestoreBrand(entities.BrandState{ID: 1, Name: "Brand", Slug: "unit-brand", Status: entities.BrandDraft, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	p, err := entities.RestoreProduct(entities.ProductState{ID: 1, BrandID: 1, Name: "Product", Description: "Complete", PriceKopecks: 100, Category: entities.CategoryHoodie, Materials: []string{"cotton"}, Status: entities.ProductDraft, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := entities.NewOwnerMembership(1, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	tx := &variantUnitTx{member: owner, product: p, data: entities.ProductPublication{Colors: []entities.ProductColor{}, Variants: []entities.ProductVariant{}}}
	return NewManagement(variantUnitRepo{tx: tx, brand: b}), tx
}
func TestVariantManagementAuthorizationAndNormalizedDuplicates(t *testing.T) {
	ctx := context.Background()
	m, tx := variantUnitManagement(t)
	c, err := m.CreateColor(ctx, 1, 1, " Black ")
	if err != nil || c.Color() != "Black" {
		t.Fatal(c, err)
	}
	before := tx.writes
	if _, err = m.CreateColor(ctx, 1, 1, " bLaCk "); !errors.Is(err, ErrConflict) || tx.writes != before {
		t.Fatal("duplicate reached write", err)
	}
	if _, err = m.CreateColor(ctx, 1, 1, "  "); !errors.Is(err, entities.ErrInvalidValue) || tx.writes != before {
		t.Fatal(err)
	}
	tx.member = nil
	if _, err = m.ProductColors(ctx, 1, 1); !errors.Is(err, ErrResourceHidden) {
		t.Fatal(err)
	}
	if _, err = m.CreateColor(ctx, 1, 1, "White"); !errors.Is(err, ErrResourceHidden) || tx.writes != before {
		t.Fatal("unauthorized write", err)
	}
	if _, err = m.ProductColors(ctx, 0, 1); !errors.Is(err, ErrForbidden) {
		t.Fatal(err)
	}
}
func TestPublishedColorCreationRollsBackIncompleteAggregate(t *testing.T) {
	ctx := context.Background()
	m, tx := variantUnitManagement(t)
	c, err := entities.RestoreProductColor(entities.ProductColorState{ID: 1, ProductID: 1, Color: "Black", Images: []string{"https://example.com/a.jpg"}})
	if err != nil {
		t.Fatal(err)
	}
	v, err := entities.RestoreProductVariant(entities.ProductVariantState{ID: 1, ProductColorID: 1, Size: "M", Stock: 1})
	if err != nil {
		t.Fatal(err)
	}
	tx.data = entities.ProductPublication{Colors: []entities.ProductColor{*c}, Variants: []entities.ProductVariant{*v}}
	if err = tx.product.Publish(tx.data, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err = m.CreateColor(ctx, 1, 1, "White"); !errors.Is(err, entities.ErrNotReady) || len(tx.data.Colors) != 1 || tx.writes != 0 {
		t.Fatal("incomplete published aggregate committed", err)
	}
}
func TestVariantStockConversionAndHierarchy(t *testing.T) {
	if _, err := stockAsInt(-1); !errors.Is(err, entities.ErrInvalidValue) {
		t.Fatal(err)
	}
	if got, err := stockAsInt(int64(math.MaxInt)); err != nil || got != math.MaxInt {
		t.Fatal(got, err)
	}
	v, err := entities.NewProductVariant(2, " One Size ", 0, "unrestricted sku")
	if err != nil {
		t.Fatal(err)
	}
	other, err := entities.RestoreProductVariant(entities.ProductVariantState{ID: 5, ProductColorID: 2, Size: "one size", Stock: 0})
	if err != nil {
		t.Fatal(err)
	}
	if err = uniqueSize(entities.ProductPublication{Variants: []entities.ProductVariant{*other}}, *v); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if _, err = managedVariant(entities.ProductPublication{Variants: []entities.ProductVariant{*other}}, 3, 5); !errors.Is(err, ErrResourceHidden) {
		t.Fatal("variant hierarchy bypass", err)
	}
}
