package entities

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

type ProductStatus string

const (
	ProductDraft     ProductStatus = "draft"
	ProductPublished ProductStatus = "published"
	ProductHidden    ProductStatus = "hidden"
)

type ProductCategory string

const (
	CategoryTShirt      ProductCategory = "tshirt"
	CategoryHoodie      ProductCategory = "hoodie"
	CategoryPants       ProductCategory = "pants"
	CategoryOuterwear   ProductCategory = "outerwear"
	CategoryFootwear    ProductCategory = "footwear"
	CategoryAccessories ProductCategory = "accessories"
	CategoryOther       ProductCategory = "other"
)

type ProductState struct {
	ID, BrandID          int64
	Name, Description    string
	PriceKopecks         int64
	Category             ProductCategory
	Materials            []string
	Status               ProductStatus
	CreatedAt, UpdatedAt time.Time
}

type Product struct{ state ProductState }

func NewProduct(brandID int64, name, description string, priceKopecks int64, category ProductCategory, materials []string, now time.Time) (*Product, error) {
	s := normalizeProduct(ProductState{BrandID: brandID, Name: name, Description: description, PriceKopecks: priceKopecks, Category: category, Materials: materials, Status: ProductDraft, CreatedAt: now, UpdatedAt: now})
	if err := validateProduct(s); err != nil {
		return nil, err
	}
	return &Product{state: s}, nil
}

// RestoreProduct validates only the product's own stored fields. Loading the
// complete aggregate and checking cross-object invariants is the caller's job.
func RestoreProduct(s ProductState) (*Product, error) {
	if s.ID <= 0 {
		return nil, invalid("product.id")
	}
	s = normalizeProduct(s)
	if err := validateProduct(s); err != nil {
		return nil, err
	}
	return &Product{state: s}, nil
}

func normalizeProduct(s ProductState) ProductState {
	s.Name = strings.TrimSpace(s.Name)
	s.Description = strings.TrimSpace(s.Description)
	s.Materials = slices.Clone(s.Materials)
	for i, material := range s.Materials {
		s.Materials[i] = strings.TrimSpace(material)
	}
	return s
}

func validateProduct(s ProductState) error {
	if s.ID < 0 || s.BrandID <= 0 {
		return invalid("product.ids")
	}
	if !validText(s.Name, 2, 120) {
		return invalid("product.name")
	}
	if !validText(s.Description, 0, 5000) {
		return invalid("product.description")
	}
	if s.PriceKopecks < 0 || (s.Status == ProductPublished && s.PriceKopecks == 0) {
		return invalid("product.price")
	}
	switch s.Category {
	case "", CategoryTShirt, CategoryHoodie, CategoryPants, CategoryOuterwear, CategoryFootwear, CategoryAccessories, CategoryOther:
	default:
		return invalid("product.category")
	}
	if len(s.Materials) > 10 {
		return invalid("product.materials")
	}
	seen := make(map[string]bool, len(s.Materials))
	for _, m := range s.Materials {
		key := strings.TrimSpace(m)
		if !nonblankText(m) || seen[key] {
			return invalid("product.material")
		}
		seen[key] = true
	}
	if !validTimes(s.CreatedAt, s.UpdatedAt) {
		return invalid("product.timestamps")
	}
	switch s.Status {
	case ProductDraft, ProductHidden:
	case ProductPublished:
		return validateProductPublicationContent(s)
	default:
		return invalid("product.status")
	}
	return nil
}

func validateProductPublicationContent(s ProductState) error {
	if !nonblankText(s.Description) || s.Category == "" || len(s.Materials) == 0 {
		return fmt.Errorf("product requires description, category and materials: %w", ErrNotReady)
	}
	return nil
}

func (p *Product) change(s ProductState, now time.Time) error {
	if err := checkTime(now, p.state.UpdatedAt); err != nil {
		return err
	}
	s = normalizeProduct(s)
	s.UpdatedAt = now
	if err := validateProduct(s); err != nil {
		return err
	}
	p.state = s
	return nil
}

func (p *Product) ChangeName(v string, now time.Time) error {
	s := p.state
	s.Name = v
	return p.change(s, now)
}

// Touch records an atomic change to this product's related data.
func (p *Product) Touch(now time.Time) error { return p.change(p.State(), now) }
func (p *Product) ChangeDescription(v string, now time.Time) error {
	s := p.state
	s.Description = v
	return p.change(s, now)
}
func (p *Product) ChangePrice(v int64, now time.Time) error {
	s := p.state
	s.PriceKopecks = v
	return p.change(s, now)
}
func (p *Product) ChangeCategory(v ProductCategory, now time.Time) error {
	s := p.state
	s.Category = v
	return p.change(s, now)
}
func (p *Product) ChangeMaterials(v []string, now time.Time) error {
	s := p.state
	s.Materials = v
	return p.change(s, now)
}

// ProductPublication must contain the COMPLETE current collection of persisted
// colors and variants. Each color needs an image and at least one size; zero
// stock is allowed. Usecase loads a consistent snapshot, checks brand activity
// and permissions, and atomically persists publication and related edits.
type ProductPublication struct {
	Colors   []ProductColor
	Variants []ProductVariant
}

func (p Product) ValidatePublication(data ProductPublication) error {
	if err := validateProduct(p.state); err != nil {
		return err
	}
	if p.ID() <= 0 || p.PriceKopecks() <= 0 || len(data.Colors) == 0 {
		return ErrNotReady
	}
	if err := validateProductPublicationContent(p.state); err != nil {
		return err
	}
	if err := ValidateProductVariants(data.Variants); err != nil {
		return err
	}
	colors := make(map[int64]bool, len(data.Colors))
	colorNames := make(map[string]bool, len(data.Colors))
	for _, c := range data.Colors {
		if err := validateProductColor(c.state); err != nil {
			return err
		}
		name := normalizedTextKey(c.Color())
		if c.ID() <= 0 || c.ProductID() != p.ID() || len(c.state.Images) == 0 || colors[c.ID()] || colorNames[name] {
			return fmt.Errorf("product colors: %w", ErrNotReady)
		}
		colors[c.ID()] = true
		colorNames[name] = true
	}
	counts := make(map[int64]int, len(colors))
	for _, v := range data.Variants {
		if v.ID() <= 0 || !colors[v.ProductColorID()] {
			return fmt.Errorf("product variants: %w", ErrNotReady)
		}
		counts[v.ProductColorID()]++
	}
	for id := range colors {
		if counts[id] == 0 {
			return fmt.Errorf("color requires a size: %w", ErrNotReady)
		}
	}
	return nil
}

func (p *Product) Publish(data ProductPublication, now time.Time) error {
	if p.state.Status != ProductDraft && p.state.Status != ProductHidden {
		return ErrInvalidTransition
	}
	if err := p.ValidatePublication(data); err != nil {
		return err
	}
	s := p.state
	s.Status = ProductPublished
	return p.change(s, now)
}

func (p *Product) Hide(now time.Time) error {
	if p.state.Status != ProductPublished {
		return ErrInvalidTransition
	}
	s := p.state
	s.Status = ProductHidden
	return p.change(s, now)
}

func (p *Product) Unpublish(now time.Time) error {
	if p.state.Status != ProductPublished && p.state.Status != ProductHidden {
		return ErrInvalidTransition
	}
	s := p.state
	s.Status = ProductDraft
	return p.change(s, now)
}

func (p Product) State() ProductState {
	s := p.state
	s.Materials = slices.Clone(s.Materials)
	return s
}
func (p Product) ID() int64                 { return p.state.ID }
func (p Product) BrandID() int64            { return p.state.BrandID }
func (p Product) Name() string              { return p.state.Name }
func (p Product) Description() string       { return p.state.Description }
func (p Product) PriceKopecks() int64       { return p.state.PriceKopecks }
func (p Product) Category() ProductCategory { return p.state.Category }
func (p Product) Materials() []string       { return slices.Clone(p.state.Materials) }
func (p Product) Status() ProductStatus     { return p.state.Status }
func (p Product) CreatedAt() time.Time      { return p.state.CreatedAt }
func (p Product) UpdatedAt() time.Time      { return p.state.UpdatedAt }
