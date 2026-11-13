package usecase

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"

	"srez/internal/entities"
)

var ErrInvalidProductQuery = errors.New("invalid product query")

const (
	DefaultProductLimit = 20
	MaxProductLimit     = 100
)

// ProductQuery describes a public catalog selection. Prices are in kopecks.
type ProductQuery struct {
	Category           entities.ProductCategory
	BrandID            int64
	MinPrice, MaxPrice *int64
	Size, Color, Sort  string
	Page               int64
	Limit              int
}

func NormalizeProductQuery(q ProductQuery) (ProductQuery, error) {
	invalid := func(field string) (ProductQuery, error) {
		return ProductQuery{}, fmt.Errorf("%w: %s", ErrInvalidProductQuery, field)
	}
	switch q.Category {
	case "", entities.CategoryTShirt, entities.CategoryHoodie, entities.CategoryPants,
		entities.CategoryOuterwear, entities.CategoryFootwear, entities.CategoryAccessories, entities.CategoryOther:
	default:
		return invalid("category")
	}
	if q.BrandID < 0 {
		return invalid("brand_id")
	}
	if (q.MinPrice != nil && *q.MinPrice < 0) || (q.MaxPrice != nil && *q.MaxPrice < 0) ||
		(q.MinPrice != nil && q.MaxPrice != nil && *q.MinPrice > *q.MaxPrice) {
		return invalid("price range")
	}
	for _, value := range []*string{&q.Size, &q.Color} {
		*value = strings.TrimSpace(*value)
		if !utf8.ValidString(*value) || strings.ContainsFunc(*value, unicode.IsControl) {
			return invalid("size or color")
		}
		*value = strings.ToLower(*value)
	}
	if utf8.RuneCountInString(q.Size) > 20 {
		return invalid("size")
	}
	if q.Sort == "" {
		q.Sort = "id_asc"
	}
	switch q.Sort {
	case "id_asc", "price_asc", "price_desc", "newest", "oldest":
	default:
		return invalid("sort")
	}
	if q.Page == 0 {
		q.Page = 1
	}
	if q.Limit == 0 {
		q.Limit = DefaultProductLimit
	}
	if q.Page < 1 || q.Limit < 1 || q.Limit > MaxProductLimit || (q.Page-1) > math.MaxInt64/int64(q.Limit) {
		return invalid("pagination")
	}
	return q, nil
}

// Catalog read models are independent of HTTP DTOs and do not expose SQL rows.
type ProductCard struct {
	Product              entities.Product
	BrandName, BrandSlug string
	PreviewImageURL      string
	Colors               []ProductCardColor
}

type ProductCardColor struct {
	ID              int64
	Color           string
	PreviewImageURL string
	Variants        []ProductCardVariant
}

type ProductCardVariant struct {
	ID    int64
	Size  string
	Stock int64
}

type ProductPage struct {
	Items []ProductCard
	Total int64
	Page  int64
	Limit int
}

func (p *Products) Browse(ctx context.Context, query ProductQuery) (*ProductPage, error) {
	query, err := NormalizeProductQuery(query)
	if err != nil {
		return nil, err
	}
	return p.repo.BrowsePublic(ctx, query)
}
