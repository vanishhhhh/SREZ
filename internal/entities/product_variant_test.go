package entities_test

import (
	"srez/internal/entities"
	"strings"
	"testing"
)

func variant(t *testing.T) *entities.ProductVariant {
	t.Helper()
	v, err := entities.NewProductVariant(20, "M", 1, "")
	noError(t, err)
	return v
}

func TestNewProductVariant(t *testing.T) {
	for _, tt := range []struct {
		name  string
		color int64
		size  string
		stock int
		sku   string
		valid bool
	}{
		{"zero stock", 20, "M", 0, "", true}, {"unicode max", 20, strings.Repeat("я", 20), 1, "SKU", true},
		{"color", 0, "M", 1, "", false}, {"size empty", 20, "", 1, "", false}, {"size whitespace", 20, " ", 1, "", false},
		{"size long", 20, strings.Repeat("я", 21), 1, "", false}, {"stock", 20, "M", -1, "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v, err := entities.NewProductVariant(tt.color, tt.size, tt.stock, tt.sku)
			if !tt.valid {
				if err == nil || v != nil {
					t.Fatal("invalid variant accepted")
				}
				return
			}
			noError(t, err)
			same(t, v.State(), entities.ProductVariantState{ProductColorID: tt.color, Size: tt.size, Stock: tt.stock, SKU: tt.sku})
			same(t, v.ID(), int64(0))
			same(t, v.ProductColorID(), tt.color)
			same(t, v.Size(), tt.size)
			same(t, v.Stock(), tt.stock)
			same(t, v.SKU(), tt.sku)
		})
	}
}

func TestProductVariantChanges(t *testing.T) {
	for _, tt := range []struct {
		name  string
		op    func(*entities.ProductVariant) error
		valid bool
		size  string
		stock int
		sku   string
	}{
		{"size", func(v *entities.ProductVariant) error { return v.ChangeSize("XL") }, true, "XL", 1, ""},
		{"stock", func(v *entities.ProductVariant) error { return v.ChangeStock(0) }, true, "M", 0, ""},
		{"sku", func(v *entities.ProductVariant) error { return v.ChangeSKU("sku") }, true, "M", 1, "sku"},
		{"empty size", func(v *entities.ProductVariant) error { return v.ChangeSize("") }, false, "", 0, ""},
		{"long size", func(v *entities.ProductVariant) error { return v.ChangeSize(strings.Repeat("я", 21)) }, false, "", 0, ""},
		{"negative stock", func(v *entities.ProductVariant) error { return v.ChangeStock(-1) }, false, "", 0, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v := variant(t)
			before := v.State()
			err := tt.op(v)
			if !tt.valid {
				unchanged(t, before, v.State(), err, entities.ErrInvalidValue)
				return
			}
			noError(t, err)
			same(t, v.Size(), tt.size)
			same(t, v.Stock(), tt.stock)
			same(t, v.SKU(), tt.sku)
		})
	}
}

func TestVariantCollection(t *testing.T) {
	for _, tt := range []struct {
		name    string
		colorID int64
		size    string
		valid   bool
	}{
		{"same color same size", 20, "M", false}, {"same color different size", 20, "L", true}, {"different color same size", 21, "M", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a := variant(t)
			b, err := entities.NewProductVariant(tt.colorID, tt.size, 0, "")
			noError(t, err)
			err = entities.ValidateProductVariants([]entities.ProductVariant{*a, *b})
			if (err == nil) != tt.valid {
				t.Fatalf("error = %v", err)
			}
		})
	}
	noError(t, entities.ValidateProductVariants(nil))
	if err := entities.ValidateProductVariants([]entities.ProductVariant{{}}); err == nil {
		t.Fatal("zero variant accepted")
	}
	s := variant(t).State()
	s.ID = 30
	v, err := entities.RestoreProductVariant(s)
	noError(t, err)
	same(t, v.ID(), int64(30))
	s.Size = "L"
	w, err := entities.RestoreProductVariant(s)
	noError(t, err)
	if err := entities.ValidateProductVariants([]entities.ProductVariant{*v, *w}); err == nil {
		t.Fatal("duplicate ID accepted")
	}
	for _, mutate := range []func(*entities.ProductVariantState){func(s *entities.ProductVariantState) { s.ID = 0 }, func(s *entities.ProductVariantState) { s.ProductColorID = 0 }, func(s *entities.ProductVariantState) { s.Stock = -1 }} {
		s := v.State()
		mutate(&s)
		if _, err := entities.RestoreProductVariant(s); err == nil {
			t.Fatal("invalid restore accepted")
		}
	}
}

func TestVariantSizeNormalization(t *testing.T) {
	for _, tt := range []struct{ input, want string }{
		{" \tM\u00a0", "M"}, {" m ", "m"}, {" One Size ", "One Size"},
		{" 44 EU ", "44 EU"}, {" Авторский-7 ", "Авторский-7"},
		{" " + strings.Repeat("я", 20) + " ", strings.Repeat("я", 20)},
	} {
		t.Run(tt.input, func(t *testing.T) {
			v, err := entities.NewProductVariant(20, tt.input, 1, "sku")
			noError(t, err)
			same(t, v.Size(), tt.want)
			v = variant(t)
			noError(t, v.ChangeSize(tt.input))
			same(t, v.Size(), tt.want)
			s := v.State()
			s.ID = 30
			s.Size = tt.input
			v, err = entities.RestoreProductVariant(s)
			noError(t, err)
			same(t, v.Size(), tt.want)
		})
	}
	for _, input := range []string{" \t\u00a0", " " + strings.Repeat("я", 21) + " "} {
		v := variant(t)
		before := v.State()
		err := v.ChangeSize(input)
		unchanged(t, before, v.State(), err, entities.ErrInvalidValue)
		if _, err := entities.NewProductVariant(20, input, 1, ""); err == nil {
			t.Fatal("invalid normalized size accepted")
		}
	}
}

func TestVariantCollectionNormalizedSizes(t *testing.T) {
	for _, tt := range []struct {
		first, second string
		colorID       int64
		duplicate     bool
	}{
		{"M", " M ", 20, true}, {"M", "m", 20, true}, {" xl ", "XL", 20, true},
		{"One Size", " one size ", 20, true}, {"44 EU", " 44 eu ", 20, true},
		{"Авторский", " авторский ", 20, true},
		{"M", " m ", 21, false}, {"One Size", "44 EU", 20, false},
	} {
		t.Run(tt.first+"/"+tt.second, func(t *testing.T) {
			a, err := entities.NewProductVariant(20, tt.first, 1, "")
			noError(t, err)
			b, err := entities.NewProductVariant(tt.colorID, tt.second, 1, "")
			noError(t, err)
			beforeA, beforeB := a.State(), b.State()
			err = entities.ValidateProductVariants([]entities.ProductVariant{*a, *b})
			if tt.duplicate {
				unchanged(t, beforeA, a.State(), err, entities.ErrInvalidValue)
			} else {
				noError(t, err)
			}
			same(t, a.State(), beforeA)
			same(t, b.State(), beforeB)
		})
	}
}
