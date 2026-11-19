package entities

import "testing"

func TestBrandPageValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*BrandPageConfig)
	}{
		{"version", func(p *BrandPageConfig) { p.SchemaVersion = 4 }},
		{"duplicate ID", func(p *BrandPageConfig) { p.Blocks[1].ID = p.Blocks[0].ID }},
		{"invalid ID size", func(p *BrandPageConfig) { p.Blocks[0].ID += "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" }},
		{"layout", func(p *BrandPageConfig) { p.Blocks[0].Layout = "grid" }},
		{"unknown type", func(p *BrandPageConfig) { p.Blocks[3].Type = "gallery" }},
		{"missing products", func(p *BrandPageConfig) { p.Blocks = append(p.Blocks[:2], p.Blocks[3]) }},
		{"too many blocks", func(p *BrandPageConfig) { p.Blocks = append(p.Blocks, p.Blocks[3]) }},
		{"untrusted source", func(p *BrandPageConfig) { p.Blocks[0].Content.Source = "https://example.test/a" }},
		{"invalid appearance", func(p *BrandPageConfig) { p.Appearance.TextColor = "url(https://example.test/a)" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page := DefaultBrandPage(BrandAppearance{})
			tc.change(&page)
			if page.Validate() == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
	page := DefaultBrandPage(BrandAppearance{TextColor: "#ffffffff"})
	page.Blocks = page.Blocks[:3]
	page.Blocks[0], page.Blocks[2] = page.Blocks[2], page.Blocks[0]
	if err := page.Validate(); err != nil {
		t.Fatal(err)
	}
}
