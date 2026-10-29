package entities

import (
	"slices"
	"strings"
)

type ProductColorState struct {
	ID, ProductID int64
	Color         string
	Images        []string
}

type ProductColor struct{ state ProductColorState }

func NewProductColor(productID int64, color string, images []string) (*ProductColor, error) {
	s := ProductColorState{ProductID: productID, Color: strings.TrimSpace(color), Images: images}
	if err := validateProductColor(s); err != nil {
		return nil, err
	}
	s.Images = slices.Clone(s.Images)
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
	return &ProductColor{state: s}, nil
}

func validateProductColor(s ProductColorState) error {
	if s.ID < 0 || s.ProductID <= 0 {
		return invalid("product_color.ids")
	}
	if !nonblankText(s.Color) {
		return invalid("product_color.color")
	}
	if len(s.Images) > 10 {
		return invalid("product_color.images")
	}
	seen := make(map[string]bool, len(s.Images))
	for _, img := range s.Images {
		if !validURL(img, false) || seen[img] {
			return invalid("product_color.image")
		}
		seen[img] = true
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
	return s
}
func (c ProductColor) ID() int64        { return c.state.ID }
func (c ProductColor) ProductID() int64 { return c.state.ProductID }
func (c ProductColor) Color() string    { return c.state.Color }
func (c ProductColor) Images() []string { return slices.Clone(c.state.Images) }
