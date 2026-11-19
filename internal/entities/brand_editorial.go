package entities

import (
	"slices"
	"unicode"
	"unicode/utf8"
)

const (
	BrandPageImageBlock BrandPageBlockType = "image"
	BrandPagePhotoText  BrandPageBlockType = "photo-text"
	BrandPageHorizontal BrandPageBlockType = "horizontal-gallery"
	BrandPageVertical   BrandPageBlockType = "vertical-gallery"
	BrandPageGrid       BrandPageBlockType = "editorial-grid"
	BrandPageText       BrandPageBlockType = "text"
)

type BrandPageCrop struct {
	X *float64 `json:"x"`
	Y *float64 `json:"y"`
}

type BrandPageImage struct {
	MediaID    int64          `json:"media_id"`
	Ratio      string         `json:"ratio"`
	Alt        string         `json:"alt,omitempty"`
	Crop       *BrandPageCrop `json:"crop,omitempty"`
	MobileCrop *BrandPageCrop `json:"mobile_crop,omitempty"`
}

func (p BrandPageConfig) MediaIDs() []int64 {
	var ids []int64
	for _, block := range p.Blocks {
		for _, image := range block.Content.Images {
			ids = append(ids, image.MediaID)
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}

func (b BrandPageBlock) systemBlock() bool {
	switch b.Type {
	case BrandPageHero, BrandPageStory, BrandPageProducts, BrandPageContacts:
		return true
	}
	return false
}

func pageText(text string, maxRunes int) bool {
	if utf8.RuneCountInString(text) > maxRunes {
		return false
	}
	for _, r := range text {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return false
		}
	}
	return true
}

func pagePreset(value string, options ...string) bool { return slices.Contains(options, value) }

func (c *BrandPageCrop) valid() bool {
	return c == nil || c.X != nil && c.Y != nil && *c.X >= 0 && *c.X <= 1 && *c.Y >= 0 && *c.Y <= 1
}

func (i BrandPageImage) valid() bool {
	return i.MediaID > 0 && i.MediaID <= MaxBrandPageRevision &&
		pagePreset(i.Ratio, "", "square", "portrait", "landscape", "wide") &&
		i.Crop.valid() && i.MobileCrop.valid() && pageText(i.Alt, 160)
}

func (b BrandPageBlock) validForVersion(version int) bool {
	if c := b.Composition; c != nil {
		hasText := b.Type == BrandPageStory || b.Type == BrandPagePhotoText || b.Type == BrandPageText
		if version != BrandPageCompositionVersion ||
			!pagePreset(c.Spacing, "", "compact", "default", "spacious") ||
			!pagePreset(c.Vertical, "", "top", "center", "bottom") ||
			(c.Vertical != "" && b.Type != BrandPageStory && b.Type != BrandPagePhotoText) ||
			!pagePreset(c.TextWidth, "", "narrow", "regular", "wide") || (c.TextWidth != "" && !hasText) {
			return false
		}
	}
	if version == BrandPageCompositionVersion && b.Type == BrandPageStory {
		c := b.Content
		return c.Source == "brand_story" && pagePreset(string(b.Layout), "text", "centered", "split", "split-reversed", "editorial", "editorial-reversed", "stacked") &&
			pagePreset(c.Align, "", "start", "center", "end") && len(c.Images) <= 1 &&
			(len(c.Images) == 0 || b.Layout != "text" && c.Images[0].valid()) &&
			c.Title == "" && c.Text == "" && c.Size == "" && c.Gap == "" && c.Columns == ""
	}

	c := b.Content
	if b.systemBlock() {
		if !b.validComposition() || len(c.Images) != 0 || c.Title != "" || c.Text != "" || c.Align != "" || c.Gap != "" || c.Columns != "" {
			return false
		}
		return c.Size == "" || version >= BrandPageEditorialVersion && b.Type == BrandPageHero && pagePreset(c.Size, "compact", "medium", "large", "viewport")
	}
	if version < BrandPageEditorialVersion || c.Source != "editorial" || !pageText(c.Title, 100) || !pageText(c.Text, 1200) {
		return false
	}
	n := len(c.Images)
	if n > 8 {
		return false
	}
	for _, i := range c.Images {
		if !i.valid() {
			return false
		}
	}
	align := pagePreset(c.Align, "", "start", "center", "end")
	size := pagePreset(c.Size, "", "compact", "medium", "large", "viewport")
	gap := pagePreset(c.Gap, "", "tight", "normal", "loose")
	switch b.Type {
	case BrandPageImageBlock:
		return n == 1 && pagePreset(string(b.Layout), "full-width", "contained", "portrait", "landscape") && align && size && c.Title == "" && c.Gap == "" && c.Columns == ""
	case BrandPagePhotoText:
		return n == 1 && (pagePreset(string(b.Layout), "photo-left", "photo-right", "stacked") || version == BrandPageCompositionVersion && b.Layout == "text-first") && align && pagePreset(c.Columns, "", "equal", "wide-photo", "wide-text") && c.Size == "" && c.Gap == ""
	case BrandPageHorizontal:
		return n >= 2 && pagePreset(string(b.Layout), "carousel") && c.Align == "" && pagePreset(c.Size, "", "compact", "medium", "large") && gap && c.Columns == "" && c.Text == ""
	case BrandPageVertical:
		return n >= 2 && b.Layout == "stacked" && c.Align == "" && c.Size == "" && gap && c.Columns == "" && c.Text == ""
	case BrandPageGrid:
		need := 3
		if b.Layout == "diptych" || b.Layout == "unequal" {
			need = 2
		}
		return n == need && (pagePreset(string(b.Layout), "feature", "diptych", "wide-top", "asymmetric") || version == BrandPageCompositionVersion && pagePreset(string(b.Layout), "unequal", "staggered")) && c.Align == "" && c.Size == "" && gap && c.Columns == "" && c.Text == ""
	case BrandPageText:
		return n == 0 && b.Layout == "text" && align && (c.Title != "" || c.Text != "") && c.Size == "" && c.Gap == "" && c.Columns == ""
	}
	return false
}
