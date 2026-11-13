package usecase

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
)

type browsingRepoSpy struct {
	productRepoStub
	query ProductQuery
	calls int
	page  *ProductPage
}

func (r *browsingRepoSpy) BrowsePublic(_ context.Context, q ProductQuery) (*ProductPage, error) {
	r.calls++
	r.query = q
	return r.page, r.err
}

func TestBrowseValidatesBeforeRepository(t *testing.T) {
	negative, low, high := int64(-1), int64(10), int64(20)
	for _, q := range []ProductQuery{
		{Category: "not-a-category"}, {BrandID: -1}, {MinPrice: &negative}, {MaxPrice: &negative},
		{MinPrice: &high, MaxPrice: &low}, {Size: strings.Repeat("я", 21)}, {Size: "\xff"},
		{Color: "blue\x00"}, {Sort: "price; DROP TABLE products"}, {Page: -1},
		{Limit: -1}, {Limit: 101}, {Page: math.MaxInt64, Limit: 100},
	} {
		r := &browsingRepoSpy{}
		if _, err := NewProducts(r, nil).Browse(context.Background(), q); !errors.Is(err, ErrInvalidProductQuery) {
			t.Fatalf("query %#v: error = %v", q, err)
		}
		if r.calls != 0 {
			t.Fatal("invalid query reached storage")
		}
	}
}

func TestBrowseDefaultsNormalizationAndErrors(t *testing.T) {
	want := &ProductPage{Items: []ProductCard{}, Page: 1, Limit: 20}
	r := &browsingRepoSpy{page: want}
	got, err := NewProducts(r, nil).Browse(context.Background(), ProductQuery{Size: " M ", Color: " ЧЁРНЫЙ "})
	if err != nil || got != want || r.query.Size != "m" || r.query.Color != "чёрный" || r.query.Page != 1 || r.query.Limit != 20 || r.query.Sort != "id_asc" {
		t.Fatalf("unexpected result: page=%#v query=%#v err=%v", got, r.query, err)
	}
	r.err = errors.New("storage unavailable")
	if _, err := NewProducts(r, nil).Browse(context.Background(), ProductQuery{}); !errors.Is(err, r.err) {
		t.Fatal("storage error was not propagated")
	}
	zero := int64(0)
	if _, err := NormalizeProductQuery(ProductQuery{MinPrice: &zero, MaxPrice: &zero, Page: math.MaxInt64, Limit: 1}); err != nil {
		t.Fatalf("valid boundaries rejected: %v", err)
	}
}
