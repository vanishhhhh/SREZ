package entities_test

import (
	"errors"
	"srez/internal/entities"
	"strings"
	"testing"
	"time"
)

func newProduct(t *testing.T) *entities.Product {
	t.Helper()
	p, err := entities.NewProduct(2, "Футболка", "Описание", 100, entities.CategoryTShirt, []string{"хлопок"}, created)
	noError(t, err)
	return p
}

func readyProduct(t *testing.T) (*entities.Product, entities.ProductPublication) {
	t.Helper()
	s := newProduct(t).State()
	s.ID = 10
	p, err := entities.RestoreProduct(s)
	noError(t, err)
	c := color(t).State()
	c.ID = 20
	restoredColor, err := entities.RestoreProductColor(c)
	noError(t, err)
	v := variant(t).State()
	v.ID = 30
	restoredVariant, err := entities.RestoreProductVariant(v)
	noError(t, err)
	return p, entities.ProductPublication{Colors: []entities.ProductColor{*restoredColor}, Variants: []entities.ProductVariant{*restoredVariant}}
}

func TestNewProduct(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*entities.ProductState)
		valid  bool
	}{
		{"basic", func(s *entities.ProductState) {}, true},
		{"minimum name", func(s *entities.ProductState) { s.Name = "Яя" }, true},
		{"maximum name", func(s *entities.ProductState) { s.Name = strings.Repeat("я", 120) }, true},
		{"maximum description", func(s *entities.ProductState) { s.Description = strings.Repeat("я", 5000) }, true},
		{"zero price", func(s *entities.ProductState) { s.PriceKopecks = 0 }, true},
		{"draft without category", func(s *entities.ProductState) { s.Category = "" }, true},
		{"empty materials", func(s *entities.ProductState) { s.Materials = nil }, true},
		{"ten materials", func(s *entities.ProductState) {
			s.Materials = []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
		}, true},
		{"bad brand", func(s *entities.ProductState) { s.BrandID = 0 }, false},
		{"short name", func(s *entities.ProductState) { s.Name = "я" }, false},
		{"blank name", func(s *entities.ProductState) { s.Name = "  " }, false},
		{"long name", func(s *entities.ProductState) { s.Name = strings.Repeat("я", 121) }, false},
		{"invalid UTF8", func(s *entities.ProductState) { s.Name = "ab\xff" }, false},
		{"long description", func(s *entities.ProductState) { s.Description = strings.Repeat("я", 5001) }, false},
		{"negative price", func(s *entities.ProductState) { s.PriceKopecks = -1 }, false},
		{"bad category", func(s *entities.ProductState) { s.Category = "bad" }, false},
		{"empty material", func(s *entities.ProductState) { s.Materials = []string{" "} }, false},
		{"duplicate material", func(s *entities.ProductState) { s.Materials = []string{"cotton", "cotton"} }, false},
		{"padded duplicate", func(s *entities.ProductState) { s.Materials = []string{"cotton", " cotton "} }, false},
		{"eleven materials", func(s *entities.ProductState) {
			s.Materials = []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"}
		}, false},
		{"zero time", func(s *entities.ProductState) { s.CreatedAt = time.Time{} }, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := newProduct(t).State()
			tt.mutate(&s)
			p, err := entities.NewProduct(s.BrandID, s.Name, s.Description, s.PriceKopecks, s.Category, s.Materials, s.CreatedAt)
			if !tt.valid {
				if err == nil || p != nil {
					t.Fatal("invalid product accepted")
				}
				return
			}
			noError(t, err)
			same(t, p.ID(), int64(0))
			same(t, p.BrandID(), s.BrandID)
			same(t, p.Name(), s.Name)
			same(t, p.Description(), s.Description)
			same(t, p.PriceKopecks(), s.PriceKopecks)
			same(t, p.Category(), s.Category)
			same(t, p.Materials(), s.Materials)
			same(t, p.Status(), entities.ProductDraft)
			same(t, p.CreatedAt(), created)
			same(t, p.UpdatedAt(), created)
		})
	}
	for _, category := range []entities.ProductCategory{entities.CategoryTShirt, entities.CategoryHoodie, entities.CategoryPants, entities.CategoryOuterwear, entities.CategoryFootwear, entities.CategoryAccessories, entities.CategoryOther} {
		t.Run(string(category), func(t *testing.T) {
			_, err := entities.NewProduct(2, "Товар", "", 0, category, nil, created)
			noError(t, err)
		})
	}
}

func TestProductChanges(t *testing.T) {
	for _, tt := range []struct {
		name  string
		op    func(*entities.Product) error
		valid bool
		want  func(*entities.ProductState)
	}{
		{"name", func(p *entities.Product) error { return p.ChangeName("Худи", changed) }, true, func(s *entities.ProductState) { s.Name = "Худи" }},
		{"description", func(p *entities.Product) error { return p.ChangeDescription("Новое", changed) }, true, func(s *entities.ProductState) { s.Description = "Новое" }},
		{"price", func(p *entities.Product) error { return p.ChangePrice(0, changed) }, true, func(s *entities.ProductState) { s.PriceKopecks = 0 }},
		{"category", func(p *entities.Product) error { return p.ChangeCategory(entities.CategoryHoodie, changed) }, true, func(s *entities.ProductState) { s.Category = entities.CategoryHoodie }},
		{"materials", func(p *entities.Product) error { return p.ChangeMaterials([]string{"шерсть"}, changed) }, true, func(s *entities.ProductState) { s.Materials = []string{"шерсть"} }},
		{"bad name", func(p *entities.Product) error { return p.ChangeName("я", changed) }, false, nil},
		{"bad description", func(p *entities.Product) error { return p.ChangeDescription(strings.Repeat("я", 5001), changed) }, false, nil},
		{"bad price", func(p *entities.Product) error { return p.ChangePrice(-1, changed) }, false, nil},
		{"bad category", func(p *entities.Product) error { return p.ChangeCategory("bad", changed) }, false, nil},
		{"bad materials", func(p *entities.Product) error { return p.ChangeMaterials([]string{"a", "a"}, changed) }, false, nil},
		{"old time", func(p *entities.Product) error { return p.ChangeName("Худи", created.Add(-time.Second)) }, false, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := newProduct(t)
			before := p.State()
			err := tt.op(p)
			if !tt.valid {
				unchanged(t, before, p.State(), err, entities.ErrInvalidValue)
				return
			}
			noError(t, err)
			tt.want(&before)
			before.UpdatedAt = changed
			same(t, p.State(), before)
		})
	}
}

func TestProductTransitions(t *testing.T) {
	for _, from := range []entities.ProductStatus{entities.ProductDraft, entities.ProductPublished, entities.ProductHidden} {
		for _, action := range []string{"publish", "hide", "unpublish"} {
			t.Run(string(from)+"/"+action, func(t *testing.T) {
				p, data := readyProduct(t)
				s := p.State()
				s.Status = from
				var err error
				p, err = entities.RestoreProduct(s)
				noError(t, err)
				before := p.State()
				valid := false
				want := from
				switch action {
				case "publish":
					valid = from != entities.ProductPublished
					want = entities.ProductPublished
					err = p.Publish(data, changed)
				case "hide":
					valid = from == entities.ProductPublished
					want = entities.ProductHidden
					err = p.Hide(changed)
				case "unpublish":
					valid = from != entities.ProductDraft
					want = entities.ProductDraft
					err = p.Unpublish(changed)
				}
				if !valid {
					unchanged(t, before, p.State(), err, entities.ErrInvalidTransition)
					return
				}
				noError(t, err)
				same(t, p.Status(), want)
				same(t, p.UpdatedAt(), changed)
			})
		}
	}
}

func TestPublicationRelatedData(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*testing.T, *entities.ProductPublication)
		valid  bool
	}{
		{"ready", func(t *testing.T, d *entities.ProductPublication) {}, true},
		{"no colors", func(t *testing.T, d *entities.ProductPublication) { d.Colors = nil }, false},
		{"no variants", func(t *testing.T, d *entities.ProductPublication) { d.Variants = nil }, false},
		{"zero color", func(t *testing.T, d *entities.ProductPublication) { d.Colors[0] = entities.ProductColor{} }, false},
		{"foreign color", func(t *testing.T, d *entities.ProductPublication) {
			s := d.Colors[0].State()
			s.ProductID = 11
			c, err := entities.RestoreProductColor(s)
			noError(t, err)
			d.Colors[0] = *c
		}, false},
		{"no images", func(t *testing.T, d *entities.ProductPublication) {
			s := d.Colors[0].State()
			s.Images = nil
			c, err := entities.RestoreProductColor(s)
			noError(t, err)
			d.Colors[0] = *c
		}, false},
		{"duplicate colors", func(t *testing.T, d *entities.ProductPublication) { d.Colors = append(d.Colors, d.Colors[0]) }, false},
		{"transient color", func(t *testing.T, d *entities.ProductPublication) { d.Colors[0] = *color(t) }, false},
		{"zero variant", func(t *testing.T, d *entities.ProductPublication) { d.Variants[0] = entities.ProductVariant{} }, false},
		{"transient variant", func(t *testing.T, d *entities.ProductPublication) { d.Variants[0] = *variant(t) }, false},
		{"foreign variant", func(t *testing.T, d *entities.ProductPublication) {
			s := d.Variants[0].State()
			s.ProductColorID = 21
			v, err := entities.RestoreProductVariant(s)
			noError(t, err)
			d.Variants[0] = *v
		}, false},
		{"duplicate size", func(t *testing.T, d *entities.ProductPublication) {
			s := d.Variants[0].State()
			s.ID++
			v, err := entities.RestoreProductVariant(s)
			noError(t, err)
			d.Variants = append(d.Variants, *v)
		}, false},
		{"normalized duplicate size", func(t *testing.T, d *entities.ProductPublication) {
			s := d.Variants[0].State()
			s.ID++
			s.Size = " m\t "
			v, err := entities.RestoreProductVariant(s)
			noError(t, err)
			d.Variants = append(d.Variants, *v)
		}, false},
		{"duplicate variant ID", func(t *testing.T, d *entities.ProductPublication) {
			s := d.Variants[0].State()
			s.Size = "L"
			v, err := entities.RestoreProductVariant(s)
			noError(t, err)
			d.Variants = append(d.Variants, *v)
		}, false},
		{"second color without size", func(t *testing.T, d *entities.ProductPublication) {
			s := d.Colors[0].State()
			s.ID++
			s.Color = "Синий"
			c, err := entities.RestoreProductColor(s)
			noError(t, err)
			d.Colors = append(d.Colors, *c)
		}, false},
		{"zero stock", func(t *testing.T, d *entities.ProductPublication) { noError(t, d.Variants[0].ChangeStock(0)) }, true},
		{"second color same size", func(t *testing.T, d *entities.ProductPublication) {
			s := d.Colors[0].State()
			s.ID++
			s.Color = "Синий"
			c, err := entities.RestoreProductColor(s)
			noError(t, err)
			d.Colors = append(d.Colors, *c)
			v := d.Variants[0].State()
			v.ID++
			v.ProductColorID = s.ID
			w, err := entities.RestoreProductVariant(v)
			noError(t, err)
			d.Variants = append(d.Variants, *w)
		}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p, data := readyProduct(t)
			tt.mutate(t, &data)
			before := p.State()
			err := p.Publish(data, changed)
			if !tt.valid {
				if err == nil {
					t.Fatal("invalid publication accepted")
				}
				same(t, p.State(), before)
				return
			}
			noError(t, err)
			same(t, p.Status(), entities.ProductPublished)
		})
	}
}

func TestProductPriceAndPublicationTime(t *testing.T) {
	p, data := readyProduct(t)
	noError(t, p.ChangePrice(0, created))
	before := p.State()
	err := p.Publish(data, changed)
	unchanged(t, before, p.State(), err, entities.ErrNotReady)
	p, data = readyProduct(t)
	before = p.State()
	err = p.Publish(data, created.Add(-time.Second))
	unchanged(t, before, p.State(), err, entities.ErrInvalidValue)
	p, data = readyProduct(t)
	noError(t, p.Publish(data, created))
	before = p.State()
	err = p.ChangePrice(0, changed)
	unchanged(t, before, p.State(), err, entities.ErrInvalidValue)
	noError(t, p.ChangePrice(1, changed))
	same(t, p.PriceKopecks(), int64(1))
	noError(t, p.Hide(changed))
	noError(t, p.ChangePrice(0, changed))
	err = p.Publish(data, changed)
	if !errors.Is(err, entities.ErrNotReady) {
		t.Fatalf("error = %v", err)
	}
	p = newProduct(t)
	before = p.State()
	err = p.Publish(data, changed)
	unchanged(t, before, p.State(), err, entities.ErrNotReady)
}

func TestProductMaterialsIsolation(t *testing.T) {
	input := []string{"cotton"}
	p, err := entities.NewProduct(2, "Shirt", "", 0, entities.CategoryOther, input, created)
	noError(t, err)
	input[0] = "bad"
	same(t, p.Materials(), []string{"cotton"})
	got := p.Materials()
	got[0] = "bad"
	s := p.State()
	s.Materials[0] = "bad"
	same(t, p.Materials(), []string{"cotton"})
	input = []string{"wool"}
	noError(t, p.ChangeMaterials(input, changed))
	input[0] = "bad"
	same(t, p.Materials(), []string{"wool"})
	s = p.State()
	s.ID = 10
	restored, err := entities.RestoreProduct(s)
	noError(t, err)
	s.Materials[0] = "bad"
	same(t, restored.Materials(), []string{"wool"})
	copy := *restored
	noError(t, copy.ChangeMaterials([]string{"linen"}, changed))
	same(t, restored.Materials(), []string{"wool"})
}

func TestRestoreProduct(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*entities.ProductState)
	}{
		{"ID", func(s *entities.ProductState) { s.ID = 0 }},
		{"status", func(s *entities.ProductState) { s.Status = "bad" }},
		{"published zero price", func(s *entities.ProductState) { s.Status = entities.ProductPublished; s.PriceKopecks = 0 }},
		{"timestamps", func(s *entities.ProductState) { s.UpdatedAt = created.Add(-time.Second) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p, _ := readyProduct(t)
			s := p.State()
			tt.mutate(&s)
			if _, err := entities.RestoreProduct(s); err == nil {
				t.Fatal("invalid restore accepted")
			}
		})
	}
}

func TestProductPublicationRequiredContent(t *testing.T) {
	for _, status := range []entities.ProductStatus{entities.ProductDraft, entities.ProductHidden} {
		for _, tt := range []struct {
			name   string
			change func(*entities.Product) error
		}{
			{"description", func(p *entities.Product) error { return p.ChangeDescription("", created) }},
			{"blank description", func(p *entities.Product) error { return p.ChangeDescription(" \t\n", created) }},
			{"category", func(p *entities.Product) error { return p.ChangeCategory("", created) }},
			{"nil materials", func(p *entities.Product) error { return p.ChangeMaterials(nil, created) }},
			{"empty materials", func(p *entities.Product) error { return p.ChangeMaterials([]string{}, created) }},
		} {
			t.Run(string(status)+"/"+tt.name, func(t *testing.T) {
				p, data := readyProduct(t)
				s := p.State()
				s.Status = status
				var err error
				p, err = entities.RestoreProduct(s)
				noError(t, err)
				noError(t, tt.change(p))
				before := p.State()
				err = p.ValidatePublication(data)
				unchanged(t, before, p.State(), err, entities.ErrNotReady)
				err = p.Publish(data, changed)
				unchanged(t, before, p.State(), err, entities.ErrNotReady)
				s = p.State()
				s.Status = entities.ProductPublished
				if _, err = entities.RestoreProduct(s); !errors.Is(err, entities.ErrNotReady) {
					t.Fatalf("restored invalid publication: %v", err)
				}
			})
		}
	}
}

func TestPublishedProductCannotClearRequiredFields(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*entities.Product) error
		want   error
	}{
		{"name", func(p *entities.Product) error { return p.ChangeName(" \t", changed) }, entities.ErrInvalidValue},
		{"description", func(p *entities.Product) error { return p.ChangeDescription("", changed) }, entities.ErrNotReady},
		{"blank description", func(p *entities.Product) error { return p.ChangeDescription(" \t\n", changed) }, entities.ErrNotReady},
		{"category", func(p *entities.Product) error { return p.ChangeCategory("", changed) }, entities.ErrNotReady},
		{"unknown category", func(p *entities.Product) error { return p.ChangeCategory("unknown", changed) }, entities.ErrInvalidValue},
		{"nil materials", func(p *entities.Product) error { return p.ChangeMaterials(nil, changed) }, entities.ErrNotReady},
		{"empty materials", func(p *entities.Product) error { return p.ChangeMaterials([]string{}, changed) }, entities.ErrNotReady},
		{"blank material", func(p *entities.Product) error { return p.ChangeMaterials([]string{" \t"}, changed) }, entities.ErrInvalidValue},
		{"price", func(p *entities.Product) error { return p.ChangePrice(0, changed) }, entities.ErrInvalidValue},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p, data := readyProduct(t)
			noError(t, p.Publish(data, created))
			before := p.State()
			err := tt.change(p)
			unchanged(t, before, p.State(), err, tt.want)
			same(t, p.Status(), entities.ProductPublished)
		})
	}
}

func TestPublicationRejectsNormalizedColorDuplicates(t *testing.T) {
	for _, tt := range []struct {
		first, second string
		duplicate     bool
	}{
		{"Красный", "Красный", true},
		{"Красный", " \tКрасный\u00a0", true},
		{"Красный", "КРАСНЫЙ", true},
		{"Red", " red ", true},
		{"Красный", "Синий", false},
	} {
		t.Run(tt.first+"/"+tt.second, func(t *testing.T) {
			p, data := readyProduct(t)
			noError(t, data.Colors[0].ChangeColor(tt.first))
			s := data.Colors[0].State()
			s.ID++
			s.Color = tt.second
			c, err := entities.RestoreProductColor(s)
			noError(t, err)
			data.Colors = append(data.Colors, *c)
			v := data.Variants[0].State()
			v.ID++
			v.ProductColorID = c.ID()
			w, err := entities.RestoreProductVariant(v)
			noError(t, err)
			data.Variants = append(data.Variants, *w)
			before := p.State()
			err = p.Publish(data, changed)
			if tt.duplicate {
				unchanged(t, before, p.State(), err, entities.ErrNotReady)
				return
			}
			noError(t, err)
			same(t, p.Status(), entities.ProductPublished)
		})
	}
}

func TestProductTextNormalization(t *testing.T) {
	input := []string{" \tХлопок\u00a0"}
	p, err := entities.NewProduct(2, " \tФутболка\u00a0", " \nОписание\t", 100, entities.CategoryTShirt, input, created)
	noError(t, err)
	same(t, p.Name(), "Футболка")
	same(t, p.Description(), "Описание")
	same(t, p.Materials(), []string{"Хлопок"})
	same(t, input, []string{" \tХлопок\u00a0"})
	s := p.State()
	s.ID = 10
	s.Name = " Товар "
	s.Description = " Описание "
	s.Materials = []string{" Материал "}
	p, err = entities.RestoreProduct(s)
	noError(t, err)
	same(t, p.Name(), "Товар")
	same(t, p.Description(), "Описание")
	same(t, p.Materials(), []string{"Материал"})
	same(t, s.Materials, []string{" Материал "})
	noError(t, p.ChangeName(" Худи ", changed))
	noError(t, p.ChangeDescription(" Описание худи ", changed))
	input = []string{" Шерсть "}
	noError(t, p.ChangeMaterials(input, changed))
	same(t, p.Name(), "Худи")
	same(t, p.Description(), "Описание худи")
	same(t, p.Materials(), []string{"Шерсть"})
	same(t, input, []string{" Шерсть "})
	before := p.State()
	input = []string{" Шерсть ", "Шерсть"}
	err = p.ChangeMaterials(input, changed.Add(time.Hour))
	unchanged(t, before, p.State(), err, entities.ErrInvalidValue)
	same(t, input, []string{" Шерсть ", "Шерсть"})
	for _, name := range []string{" я ", " "} {
		before := p.State()
		err := p.ChangeName(name, changed.Add(time.Hour))
		unchanged(t, before, p.State(), err, entities.ErrInvalidValue)
	}
}
