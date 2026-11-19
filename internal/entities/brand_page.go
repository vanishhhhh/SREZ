package entities

import "regexp"

const BrandPageVersion = 1
const BrandPageEditorialVersion = 2
const BrandPageCompositionVersion = 3
const MaxBrandPageRevision = 9007199254740991

type BrandPageBlockType string
type BrandPageLayout string
type BrandPageSource string

const (
	BrandPageHero     BrandPageBlockType = "hero"
	BrandPageStory    BrandPageBlockType = "story"
	BrandPageProducts BrandPageBlockType = "products"
	BrandPageContacts BrandPageBlockType = "contacts"
)

type BrandPageContent struct {
	Source  BrandPageSource  `json:"source"`
	Images  []BrandPageImage `json:"images,omitempty"`
	Title   string           `json:"title,omitempty"`
	Text    string           `json:"text,omitempty"`
	Size    string           `json:"size,omitempty"`
	Align   string           `json:"align,omitempty"`
	Gap     string           `json:"gap,omitempty"`
	Columns string           `json:"columns,omitempty"`
}

type BrandComposition struct {
	Vertical  string `json:"vertical,omitempty"`
	TextWidth string `json:"text_width,omitempty"`
	Spacing   string `json:"spacing,omitempty"`
}

type BrandPageBlock struct {
	Composition *BrandComposition  `json:"composition,omitempty"`
	ID          string             `json:"id"`
	Type        BrandPageBlockType `json:"type"`
	Layout      BrandPageLayout    `json:"layout"`
	Content     BrandPageContent   `json:"content"`
}

type BrandPageConfig struct {
	SchemaVersion int              `json:"schema_version"`
	Blocks        []BrandPageBlock `json:"blocks"`
	Appearance    BrandAppearance  `json:"appearance"`
}

// The array is the visual order. IDs are independent of placement and layout.
func DefaultBrandPage(appearance BrandAppearance) BrandPageConfig {
	return BrandPageConfig{SchemaVersion: BrandPageVersion, Appearance: appearance, Blocks: []BrandPageBlock{
		{ID: "hero", Type: BrandPageHero, Layout: "full-width", Content: BrandPageContent{Source: "brand_banner"}},
		{ID: "story", Type: BrandPageStory, Layout: "split", Content: BrandPageContent{Source: "brand_story"}},
		{ID: "products", Type: BrandPageProducts, Layout: "grid", Content: BrandPageContent{Source: "brand_catalog"}},
		{ID: "contacts", Type: BrandPageContacts, Layout: "inline", Content: BrandPageContent{Source: "brand_contacts"}},
	}}
}

var brandPageBlockID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)

func (p BrandPageConfig) Validate() error {
	if p.SchemaVersion != BrandPageVersion && p.SchemaVersion != BrandPageEditorialVersion && p.SchemaVersion != BrandPageCompositionVersion {
		return invalid("page.schema_version")
	}
	minBlocks, maxBlocks := 3, 4
	if p.SchemaVersion >= BrandPageEditorialVersion {
		maxBlocks = 20
	}
	if p.SchemaVersion == BrandPageCompositionVersion {
		minBlocks = 2
	}
	if len(p.Blocks) < minBlocks || len(p.Blocks) > maxBlocks {
		return invalid("page.blocks")
	}
	if err := p.Appearance.Validate(); err != nil {
		return err
	}
	ids := make(map[string]bool, len(p.Blocks))
	types := make(map[BrandPageBlockType]bool, len(p.Blocks))
	for _, block := range p.Blocks {
		if !brandPageBlockID.MatchString(block.ID) || ids[block.ID] {
			return invalid("page.block.id")
		}
		if block.systemBlock() && types[block.Type] || !block.validForVersion(p.SchemaVersion) {
			return invalid("page.block.composition")
		}
		ids[block.ID], types[block.Type] = true, true
	}
	if !types[BrandPageHero] || !types[BrandPageProducts] || (p.SchemaVersion < BrandPageCompositionVersion && !types[BrandPageStory]) {
		return invalid("page.required_blocks")
	}
	return nil
}

func (b BrandPageBlock) validComposition() bool {
	switch b.Type {
	case BrandPageHero:
		return b.Content.Source == "brand_banner" && (b.Layout == "full-width" || b.Layout == "split")
	case BrandPageStory:
		return b.Content.Source == "brand_story" && (b.Layout == "text" || b.Layout == "split")
	case BrandPageProducts:
		return b.Content.Source == "brand_catalog" && b.Layout == "grid"
	case BrandPageContacts:
		return b.Content.Source == "brand_contacts" && b.Layout == "inline"
	default:
		return false
	}
}

type BrandPageEditorState struct {
	MediaIDs          []int64          `json:"media_ids"`
	Draft             BrandPageConfig  `json:"draft"`
	Published         *BrandPageConfig `json:"published"`
	Revision          int64            `json:"revision"`
	PublishedRevision *int64           `json:"published_revision"`
}
