package entities

import (
	"slices"
	"strings"
)

type ProductColorState struct {
	ID, ProductID int64
	Color         string
	Images        []string
	Sources       []ImageSource
}

type ProductColor struct{ state ProductColorState }

func NewProductColor(productID int64, color string, images []string) (*ProductColor, error) {
	s := ProductColorState{ProductID: productID, Color: strings.TrimSpace(color), Images: images}
	if err := validateProductColor(s); err != nil {
		return nil, err
	}
	s.Images = slices.Clone(s.Images)
	s.Sources = slices.Clone(s.Sources)
	return &ProductColor{state: s}, nil
}

func RestoreProductColor(s ProductColorState) (*ProductColor, error) {
	if s.ID <= 0 {
		return nil, invalid("product_color.id")
	}
	s.Color = strings.TrimSpace(s.Color)
	if err := validateProductColor(s); err != nil {
		return nil, err
	}
	s.Images = slices.Clone(s.Images)
	s.Sources = slices.Clone(s.Sources)
	return &ProductColor{state: s}, nil
}

func validateProductColor(s ProductColorState) error {
	if s.ID < 0 || s.ProductID <= 0 {
		return invalid("product_color.ids")
	}
	if !nonblankText(s.Color) {
		return invalid("product_color.color")
	}
	if len(s.Images)+len(s.Sources) > 10 || (len(s.Images) > 0 && len(s.Sources) > 0) {
		return invalid("product_color.images")
	}
	seen := make(map[string]bool, len(s.Images))
	for _, img := range s.Images {
		if !validURL(img, false) || seen[img] {
			return invalid("product_color.image")
		}
		seen[img] = true
	}

	seenSources := map[ImageSource]bool{}
	for _, source := range s.Sources {
		if !source.Valid() || seenSources[source] {
			return invalid("product_color.source")
		}
		seenSources[source] = true
	}
	return nil
}

func (c *ProductColor) ChangeColor(color string) error {
	s := c.state
	s.Color = strings.TrimSpace(color)
	if err := validateProductColor(s); err != nil {
		return err
	}
	c.state = s
	return nil
}

func (c *ProductColor) AddImage(url string) error {
	s := c.State()
	s.Images = append(s.Images, url)
	if err := validateProductColor(s); err != nil {
		return err
	}
	c.state = s
	return nil
}

func (c *ProductColor) RemoveImage(url string) error {
	if err := validateProductColor(c.state); err != nil {
		return err
	}
	i := slices.Index(c.state.Images, url)
	if i < 0 {
		return invalid("product_color.image_not_found")
	}
	s := c.State()
	s.Images = slices.Delete(s.Images, i, i+1)
	c.state = s
	return nil
}

func (c ProductColor) State() ProductColorState {
	s := c.state
	s.Images = slices.Clone(s.Images)
	s.Sources = slices.Clone(s.Sources)
	return s
}
func (c ProductColor) ID() int64        { return c.state.ID }
func (c ProductColor) ProductID() int64 { return c.state.ProductID }
func (c ProductColor) Color() string    { return c.state.Color }
func (c ProductColor) Images() []string {
	if c.state.Sources == nil {
		return slices.Clone(c.state.Images)
	}
	out := []string{}
	for _, s := range c.state.Sources {
		if s.MediaID == 0 {
			out = append(out, s.URL)
		}
	}
	return out
}

// ImageSource separates HTTPS legacy imports from validated managed references.
type ImageSource struct {
	URL     string
	MediaID int64
}

func (s ImageSource) Valid() bool {
	return (s.MediaID == 0 && validURL(s.URL, false)) || (s.MediaID > 0 && s.URL == "")
}
func (c ProductColor) ImageSources() []ImageSource {
	if c.state.Sources != nil {
		return slices.Clone(c.state.Sources)
	}
	result := make([]ImageSource, 0, len(c.state.Images))
	for _, u := range c.state.Images {
		result = append(result, ImageSource{URL: u})
	}
	return result
}
