package entities

import "testing"

func TestEditorialBlocksAndCropValidation(t *testing.T) {
	config := DefaultBrandPage(BrandAppearance{})
	config.SchemaVersion = 2
	photo := BrandPageImage{MediaID: 101}
	added := BrandPageBlock{ID: "campaign-a", Type: BrandPageImageBlock, Layout: "full-width",
		Content: BrandPageContent{Source: "editorial", Images: []BrandPageImage{photo}}}
	config.Blocks = append(config.Blocks, added)
	added.ID = "campaign-b"
	config.Blocks = append(config.Blocks, added)
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	invalid := []struct {
		name   string
		change func(*BrandPageBlock)
	}{
		{"duplicate ID", func(b *BrandPageBlock) { b.ID = "campaign-a" }},
		{"layout", func(b *BrandPageBlock) { b.Layout = "arbitrary-css" }},
		{"out of bounds", func(b *BrandPageBlock) {
			x, y := -0.1, 0.5
			b.Content.Images = []BrandPageImage{{MediaID: 12, Crop: &BrandPageCrop{X: &x, Y: &y}}}
		}},
		{"missing coordinate", func(b *BrandPageBlock) {
			b.Content.Images = []BrandPageImage{{MediaID: 12, MobileCrop: &BrandPageCrop{}}}
		}},
		{"unbounded ratio", func(b *BrandPageBlock) { b.Content.Images = []BrandPageImage{{MediaID: 12, Ratio: "100 / 1"}} }},
		{"invalid media reference", func(b *BrandPageBlock) { b.Content.Images = []BrandPageImage{{MediaID: 0}} }},
		{"invalid gallery count", func(b *BrandPageBlock) { b.Type = BrandPageHorizontal; b.Layout = "carousel" }},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			config.Blocks[len(config.Blocks)-1] = added
			tt.change(&config.Blocks[len(config.Blocks)-1])
			if err := config.Validate(); err == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
}
