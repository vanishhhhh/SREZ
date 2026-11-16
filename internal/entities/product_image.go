package entities

// ProductImage has stable identity/order and exactly one source: a historical
// HTTPS URL or a managed media ID. Only the HTTP renderer creates delivery URLs.
type ProductImageState struct {
	ID, ProductColorID int64
	URL                string
	MediaID            int64
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
	if (s.MediaID == 0 && !validURL(s.URL, false)) || s.MediaID < 0 || (s.MediaID > 0 && s.URL != "") {
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
	s.MediaID = 0
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

func NewManagedProductImage(colorID, mediaID int64, position int) (*ProductImage, error) {
	s := ProductImageState{ProductColorID: colorID, MediaID: mediaID, Position: position}
	if mediaID <= 0 {
		return nil, invalid("product_image.media")
	}
	if e := validateProductImage(s); e != nil {
		return nil, e
	}
	return &ProductImage{state: s}, nil
}
func (i *ProductImage) ChangeMedia(id int64) error {
	s := i.state
	s.URL = ""
	s.MediaID = id
	if id <= 0 {
		return invalid("product_image.media")
	}
	if e := validateProductImage(s); e != nil {
		return e
	}
	i.state = s
	return nil
}
func (i ProductImage) MediaID() int64 { return i.state.MediaID }
