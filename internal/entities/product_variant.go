package entities

import "strings"

type ProductVariantState struct {
	ID, ProductColorID int64
	Size               string
	Stock              int
	SKU                string
}

type ProductVariant struct{ state ProductVariantState }

func NewProductVariant(productColorID int64, size string, stock int, sku string) (*ProductVariant, error) {
	s := ProductVariantState{ProductColorID: productColorID, Size: strings.TrimSpace(size), Stock: stock, SKU: sku}
	if err := validateProductVariant(s); err != nil {
		return nil, err
	}
	return &ProductVariant{state: s}, nil
}

func RestoreProductVariant(s ProductVariantState) (*ProductVariant, error) {
	if s.ID <= 0 {
		return nil, invalid("product_variant.id")
	}
	s.Size = strings.TrimSpace(s.Size)
	if err := validateProductVariant(s); err != nil {
		return nil, err
	}
	return &ProductVariant{state: s}, nil
}

func validateProductVariant(s ProductVariantState) error {
	if s.ID < 0 || s.ProductColorID <= 0 {
		return invalid("product_variant.ids")
	}
	if !validText(s.Size, 1, 20) {
		return invalid("product_variant.size")
	}
	if s.Stock < 0 {
		return invalid("product_variant.stock")
	}
	return nil
}

func (v *ProductVariant) change(s ProductVariantState) error {
	if err := validateProductVariant(s); err != nil {
		return err
	}
	v.state = s
	return nil
}

func (v *ProductVariant) ChangeSize(size string) error {
	s := v.state
	s.Size = strings.TrimSpace(size)
	return v.change(s)
}
func (v *ProductVariant) ChangeStock(stock int) error {
	s := v.state
	s.Stock = stock
	return v.change(s)
}
func (v *ProductVariant) ChangeSKU(sku string) error { s := v.state; s.SKU = sku; return v.change(s) }
func (v ProductVariant) State() ProductVariantState  { return v.state }
func (v ProductVariant) ID() int64                   { return v.state.ID }
func (v ProductVariant) ProductColorID() int64       { return v.state.ProductColorID }
func (v ProductVariant) Size() string                { return v.state.Size }
func (v ProductVariant) Stock() int                  { return v.state.Stock }
func (v ProductVariant) SKU() string                 { return v.state.SKU }

// ValidateProductVariants checks a collection, including transient variants.
// A storage constraint is still needed to protect concurrent writes.
func ValidateProductVariants(variants []ProductVariant) error {
	type sizeKey struct {
		colorID int64
		size    string
	}
	seen := make(map[sizeKey]bool, len(variants))
	ids := make(map[int64]bool, len(variants))
	for _, v := range variants {
		if err := validateProductVariant(v.state); err != nil {
			return err
		}
		key := sizeKey{v.ProductColorID(), normalizedTextKey(v.Size())}
		if seen[key] || (v.ID() > 0 && ids[v.ID()]) {
			return invalid("product_variants.duplicate")
		}
		seen[key] = true
		if v.ID() > 0 {
			ids[v.ID()] = true
		}
	}
	return nil
}
