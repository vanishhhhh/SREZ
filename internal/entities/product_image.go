package entities

// ProductImage supplies stable identity and ordering for management. The public
// ProductColor.Images contract remains an ordered slice of HTTPS URLs.
type ProductImageState struct {
	ID, ProductColorID int64
	URL                string
	Position           int
}

type ProductImage struct{ state ProductImageState }

func NewProductImage(colorID int64, url string, position int) (*ProductImage, error) {
	s := ProductImageState{ProductColorID: colorID, URL: url, Position: position}
	if err := validateProductImage(s); err != nil {
		return nil, err
	}
	return &ProductImage{state: s}, nil
}

func RestoreProductImage(s ProductImageState) (*ProductImage, error) {
	if s.ID <= 0 {
		return nil, invalid("product_image.id")
	}
	if err := validateProductImage(s); err != nil {
		return nil, err
	}
	return &ProductImage{state: s}, nil
}

func validateProductImage(s ProductImageState) error {
	if s.ID < 0 || s.ProductColorID <= 0 {
		return invalid("product_image.ids")
	}
	if !validURL(s.URL, false) {
		return invalid("product_image.url")
	}
	if s.Position < 0 || s.Position >= 10 {
		return invalid("product_image.position")
	}
	return nil
}

func (i *ProductImage) ChangeURL(url string) error {
	s := i.state
	s.URL = url
	if err := validateProductImage(s); err != nil {
		return err
	}
	i.state = s
	return nil
}
func (i *ProductImage) ChangePosition(position int) error {
	s := i.state
	s.Position = position
	if err := validateProductImage(s); err != nil {
		return err
	}
	i.state = s
	return nil
}
func (i ProductImage) State() ProductImageState { return i.state }
func (i ProductImage) ID() int64                { return i.state.ID }
func (i ProductImage) ProductColorID() int64    { return i.state.ProductColorID }
func (i ProductImage) URL() string              { return i.state.URL }
func (i ProductImage) Position() int            { return i.state.Position }
