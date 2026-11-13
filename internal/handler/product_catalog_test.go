package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"srez/internal/usecase"
)

type catalogBrowseSpy struct {
	*catalogRepository
	query usecase.ProductQuery
	calls int
	page  *usecase.ProductPage
}

func (r *catalogBrowseSpy) BrowsePublic(_ context.Context, q usecase.ProductQuery) (*usecase.ProductPage, error) {
	r.calls++
	r.query = q
	if r.page != nil {
		return r.page, nil
	}
	return &usecase.ProductPage{Items: []usecase.ProductCard{}, Total: 23, Page: q.Page, Limit: q.Limit}, nil
}

func catalogHandlerWithSpy(r *catalogBrowseSpy) *Handler {
	return New(usecase.NewUsers(mockUserRepository{}), usecase.NewBrands(r), usecase.NewProducts(r, r), false)
}

func TestCatalogQueryAndPageHeaders(t *testing.T) {
	r := &catalogBrowseSpy{catalogRepository: newCatalogRepository()}
	response := catalogRequest(t, catalogHandlerWithSpy(r), http.MethodGet,
		"/products?category=hoodie&brand_id=2&min_price=200000&max_price=800000&size=%20M%20&color=%D0%A7%D1%91%D1%80%D0%BD%D1%8B%D0%B9&sort=price_asc&page=2&limit=20", "")
	if response.Code != 200 || r.calls != 1 || r.query.Category != "hoodie" || r.query.BrandID != 2 ||
		r.query.MinPrice == nil || *r.query.MinPrice != 200000 || r.query.MaxPrice == nil || *r.query.MaxPrice != 800000 ||
		r.query.Size != "m" || r.query.Color != "чёрный" || r.query.Sort != "price_asc" || r.query.Page != 2 || r.query.Limit != 20 {
		t.Fatalf("query was not forwarded: %#v, status=%d", r.query, response.Code)
	}
	for name, want := range map[string]string{"X-Total-Count": "23", "X-Page": "2", "X-Limit": "20", "X-Total-Pages": "2"} {
		if response.Header().Get(name) != want {
			t.Fatalf("%s = %q, want %q", name, response.Header().Get(name), want)
		}
	}
	assertCatalogJSON(t, response.Body.Bytes(), "[]")
}

func TestCatalogQueryErrorsDoNotCallStorage(t *testing.T) {
	for _, query := range []string{
		"category=unknown", "category=", "brand_id=0", "brand_id=-1", "brand_id=abc", "brand_id=9223372036854775808",
		"min_price=-1", "max_price=-1", "min_price=2&max_price=1", "min_price=1.5", "min_price=9223372036854775808",
		"sort=price_asc%3BDROP%20TABLE%20users", "sort=", "page=0", "page=-1", "page=x", "page=1.1",
		"page=9223372036854775807&limit=100", "limit=0", "limit=-1", "limit=101", "limit=9223372036854775808",
		"size=", "size=%20", "size=%FF", "size=M&size=L", "page=1&page=2", "sort=price_asc&sort=price_desc",
		"size=" + strings.Repeat("a", 21), "color=%00", "size=%ZZ", "size=M;limit=10",
	} {
		t.Run(query, func(t *testing.T) {
			r := &catalogBrowseSpy{catalogRepository: newCatalogRepository()}
			assertCatalogError(t, catalogRequest(t, catalogHandlerWithSpy(r), http.MethodGet, "/products?"+query, ""), 400)
			if r.calls != 0 {
				t.Fatal("invalid query called storage")
			}
		})
	}
}

func TestCatalogDefaultsAndEmptyPage(t *testing.T) {
	r := &catalogBrowseSpy{catalogRepository: newCatalogRepository(), page: &usecase.ProductPage{Items: nil, Page: 1, Limit: 20}}
	response := catalogRequest(t, catalogHandlerWithSpy(r), http.MethodGet, "/products", "")
	if response.Code != 200 || r.query.Page != 1 || r.query.Limit != 20 || r.query.Sort != "id_asc" || response.Header().Get("X-Total-Pages") != "0" {
		t.Fatal("invalid defaults or empty-page metadata")
	}
	assertCatalogJSON(t, response.Body.Bytes(), "[]")
}

func TestCatalogCardsAndAvailability(t *testing.T) {
	r := newCatalogRepository()
	// No black sizes left; white still has One Size in stock.
	entry := &r.products[0]
	for i := range entry.publication.Variants {
		if entry.publication.Variants[i].ProductColorID() == entry.publication.Colors[0].ID() {
			if err := entry.publication.Variants[i].ChangeStock(0); err != nil {
				t.Fatal(err)
			}
		}
	}
	h := New(usecase.NewUsers(mockUserRepository{}), usecase.NewBrands(r), usecase.NewProducts(r, r), false)
	response := catalogRequest(t, h, http.MethodGet, "/products", "")
	var cards []productCardResponse
	if err := json.Unmarshal(response.Body.Bytes(), &cards); err != nil || len(cards) != 2 {
		t.Fatalf("cards: %s, %v", response.Body.String(), err)
	}
	first := cards[0]
	if first.PreviewImageURL != "https://example.com/products/1/black.png" || first.BrandName != "Срез Студия" || first.BrandSlug != "srez-studio" || !first.IsAvailable ||
		!reflect.DeepEqual(first.AvailableColors, []string{"Белый"}) || !reflect.DeepEqual(first.AvailableSizes, []string{"One Size"}) || first.Colors[0].IsAvailable || len(first.Colors[0].AvailableSizes) != 0 {
		t.Fatalf("incorrect availability: %#v", first)
	}
	// All variants now exhausted; the card and its image remain visible.
	for i := range entry.publication.Variants {
		if err := entry.publication.Variants[i].ChangeStock(0); err != nil {
			t.Fatal(err)
		}
	}
	response = catalogRequest(t, h, http.MethodGet, "/products", "")
	if err := json.Unmarshal(response.Body.Bytes(), &cards); err != nil {
		t.Fatal(err)
	}
	if cards[0].IsAvailable || cards[0].AvailableColors == nil || len(cards[0].AvailableColors) != 0 || cards[0].AvailableSizes == nil || len(cards[0].AvailableSizes) != 0 {
		t.Fatal("sold-out card must expose false and empty arrays")
	}
}
