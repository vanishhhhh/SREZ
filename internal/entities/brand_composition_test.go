package entities

import "testing"

func TestBrandCompositionSchemas(t *testing.T) {
	page := DefaultBrandPage(BrandAppearance{})
	story := page.Blocks[1]
	for _, version := range []int{1, 2} {
		page.SchemaVersion = version
		if err := page.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	story.Layout = "editorial-reversed"
	story.Composition = &BrandComposition{Spacing: "spacious", Vertical: "center", TextWidth: "narrow"}
	story.Content.Images = []BrandPageImage{{MediaID: 17}}
	page.Blocks[1] = story
	if page.Validate() == nil {
		t.Fatal("V2 accepted V3 composition")
	}
	page.SchemaVersion = 3
	if err := page.Validate(); err != nil {
		t.Fatal(err)
	}
	if ids := page.MediaIDs(); len(ids) != 1 || ids[0] != 17 {
		t.Fatal(ids)
	}
	story.Composition.Vertical = "absolute"
	if page.Validate() == nil {
		t.Fatal("arbitrary alignment accepted")
	}
	page.Blocks = append(page.Blocks[:1], page.Blocks[2:]...)
	if err := page.Validate(); err != nil {
		t.Fatal(err)
	}
	page.SchemaVersion = 2
	if page.Validate() == nil {
		t.Fatal("V2 accepted a missing story")
	}
}
