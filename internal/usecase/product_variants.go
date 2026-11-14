package usecase

import (
	"context"
	"math"
	"slices"
	"strings"
	"time"

	"srez/internal/entities"
)

// ProductVariantTx extends the existing brand transaction, not its locking or
// authorization policy. All operations participate in WithinBrand's commit.
type ProductVariantTx interface {
	InsertColor(context.Context, entities.ProductColor) (*entities.ProductColor, error)
	SaveColor(context.Context, entities.ProductColor) error
	DeleteColor(context.Context, int64, int64) error
	InsertVariant(context.Context, entities.ProductVariant) (*entities.ProductVariant, error)
	SaveVariant(context.Context, entities.ProductVariant) error
	DeleteVariant(context.Context, int64, int64) error
	Images(context.Context, int64) ([]entities.ProductImage, error)
	InsertImage(context.Context, entities.ProductImage) (*entities.ProductImage, error)
	SaveImage(context.Context, entities.ProductImage) error
	DeleteImage(context.Context, int64, int64) error
	SaveImagePositions(context.Context, int64, []int64) error
}

type VariantPatch struct {
	Size, SKU *string
	Stock     *int64
}
type ImagePatch struct {
	URL      *string
	Position *int
}

func (m *Management) withProduct(ctx context.Context, uid, id int64, write bool,
	fn func(ManagementTx, *entities.Product, entities.ProductPublication) error) error {
	if uid <= 0 {
		return ErrForbidden
	}
	if id <= 0 {
		return entities.ErrInvalidValue
	}
	brandID, err := m.repo.ProductBrand(ctx, id)
	if err != nil {
		return err
	}
	return m.brand(ctx, uid, brandID, false, func(tx ManagementTx, b *entities.Brand) error {
		if b.Status() == entities.BrandSuspended {
			return ErrForbidden
		}
		p, err := tx.Product(ctx, id)
		if err != nil {
			return err
		}
		if p == nil || p.BrandID() != b.ID() {
			return ErrResourceHidden
		}
		data, err := tx.Publication(ctx, id)
		if err != nil {
			return err
		}
		if err = fn(tx, p, data); err != nil {
			return err
		}
		if !write {
			return nil
		}
		// Validate the actual complete post-write aggregate while all parent and
		// child locks are still held. Any failure rolls back every SQL change.
		if p.Status() == entities.ProductPublished {
			data, err = tx.Publication(ctx, id)
			if err != nil {
				return err
			}
			if err = p.ValidatePublication(data); err != nil {
				return err
			}
		}
		now := time.Now().UTC()
		if now.Before(p.UpdatedAt()) {
			now = p.UpdatedAt()
		}
		if err = p.Touch(now); err != nil {
			return err
		}
		return tx.SaveProduct(ctx, *p)
	})
}

func managedColor(data entities.ProductPublication, id int64) (*entities.ProductColor, error) {
	for _, c := range data.Colors {
		if c.ID() == id {
			return &c, nil
		}
	}
	return nil, ErrResourceHidden
}
func managedVariant(data entities.ProductPublication, colorID, id int64) (*entities.ProductVariant, error) {
	if _, err := managedColor(data, colorID); err != nil {
		return nil, err
	}
	for _, v := range data.Variants {
		if v.ID() == id && v.ProductColorID() == colorID {
			return &v, nil
		}
	}
	return nil, ErrResourceHidden
}
func normalizedVariantKey(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func (m *Management) ProductColors(ctx context.Context, uid, productID int64) ([]entities.ProductColor, error) {
	result := []entities.ProductColor{}
	err := m.withProduct(ctx, uid, productID, false, func(_ ManagementTx, _ *entities.Product, data entities.ProductPublication) error {
		result = data.Colors
		return nil
	})
	return result, err
}
func (m *Management) CreateColor(ctx context.Context, uid, productID int64, name string) (*entities.ProductColor, error) {
	var result *entities.ProductColor
	err := m.withProduct(ctx, uid, productID, true, func(tx ManagementTx, _ *entities.Product, data entities.ProductPublication) error {
		color, err := entities.NewProductColor(productID, name, nil)
		if err != nil {
			return err
		}
		for _, c := range data.Colors {
			if normalizedVariantKey(c.Color()) == normalizedVariantKey(color.Color()) {
				return ErrConflict
			}
		}
		result, err = tx.InsertColor(ctx, *color)
		return err
	})
	return result, err
}
func (m *Management) RenameColor(ctx context.Context, uid, productID, colorID int64, name string) (*entities.ProductColor, error) {
	var result *entities.ProductColor
	err := m.withProduct(ctx, uid, productID, true, func(tx ManagementTx, _ *entities.Product, data entities.ProductPublication) error {
		c, err := managedColor(data, colorID)
		if err != nil {
			return err
		}
		if err = c.ChangeColor(name); err != nil {
			return err
		}
		for _, other := range data.Colors {
			if other.ID() != c.ID() && normalizedVariantKey(other.Color()) == normalizedVariantKey(c.Color()) {
				return ErrConflict
			}
		}
		if err = tx.SaveColor(ctx, *c); err != nil {
			return err
		}
		result = c
		return nil
	})
	return result, err
}
func (m *Management) DeleteColor(ctx context.Context, uid, productID, colorID int64) error {
	return m.withProduct(ctx, uid, productID, true, func(tx ManagementTx, _ *entities.Product, data entities.ProductPublication) error {
		if _, err := managedColor(data, colorID); err != nil {
			return err
		}
		return tx.DeleteColor(ctx, productID, colorID)
	})
}

func (m *Management) ProductVariants(ctx context.Context, uid, productID, colorID int64) ([]entities.ProductVariant, error) {
	result := []entities.ProductVariant{}
	err := m.withProduct(ctx, uid, productID, false, func(_ ManagementTx, _ *entities.Product, data entities.ProductPublication) error {
		if _, err := managedColor(data, colorID); err != nil {
			return err
		}
		for _, v := range data.Variants {
			if v.ProductColorID() == colorID {
				result = append(result, v)
			}
		}
		return nil
	})
	return result, err
}
func stockAsInt(stock int64) (int, error) {
	if stock < 0 || stock > int64(math.MaxInt) {
		return 0, entities.ErrInvalidValue
	}
	return int(stock), nil
}
func uniqueSize(data entities.ProductPublication, variant entities.ProductVariant) error {
	for _, v := range data.Variants {
		if v.ID() != variant.ID() && v.ProductColorID() == variant.ProductColorID() && normalizedVariantKey(v.Size()) == normalizedVariantKey(variant.Size()) {
			return ErrConflict
		}
	}
	return nil
}
func (m *Management) CreateVariant(ctx context.Context, uid, productID, colorID int64, size string, stock int64, sku string) (*entities.ProductVariant, error) {
	var result *entities.ProductVariant
	err := m.withProduct(ctx, uid, productID, true, func(tx ManagementTx, _ *entities.Product, data entities.ProductPublication) error {
		if _, err := managedColor(data, colorID); err != nil {
			return err
		}
		n, err := stockAsInt(stock)
		if err != nil {
			return err
		}
		v, err := entities.NewProductVariant(colorID, size, n, sku)
		if err != nil {
			return err
		}
		if err = uniqueSize(data, *v); err != nil {
			return err
		}
		result, err = tx.InsertVariant(ctx, *v)
		return err
	})
	return result, err
}
func (m *Management) ChangeVariant(ctx context.Context, uid, productID, colorID, variantID int64, patch VariantPatch) (*entities.ProductVariant, error) {
	var result *entities.ProductVariant
	err := m.withProduct(ctx, uid, productID, true, func(tx ManagementTx, _ *entities.Product, data entities.ProductPublication) error {
		v, err := managedVariant(data, colorID, variantID)
		if err != nil {
			return err
		}
		if patch.Size == nil && patch.Stock == nil && patch.SKU == nil {
			return entities.ErrInvalidValue
		}
		if patch.Size != nil {
			if err = v.ChangeSize(*patch.Size); err != nil {
				return err
			}
		}
		if patch.Stock != nil {
			n, err := stockAsInt(*patch.Stock)
			if err != nil {
				return err
			}
			if err = v.ChangeStock(n); err != nil {
				return err
			}
		}
		if patch.SKU != nil {
			if err = v.ChangeSKU(*patch.SKU); err != nil {
				return err
			}
		}
		if err = uniqueSize(data, *v); err != nil {
			return err
		}
		if err = tx.SaveVariant(ctx, *v); err != nil {
			return err
		}
		result = v
		return nil
	})
	return result, err
}
func (m *Management) DeleteVariant(ctx context.Context, uid, productID, colorID, variantID int64) error {
	return m.withProduct(ctx, uid, productID, true, func(tx ManagementTx, _ *entities.Product, data entities.ProductPublication) error {
		if _, err := managedVariant(data, colorID, variantID); err != nil {
			return err
		}
		return tx.DeleteVariant(ctx, colorID, variantID)
	})
}

func (m *Management) ProductImages(ctx context.Context, uid, productID, colorID int64) ([]entities.ProductImage, error) {
	result := []entities.ProductImage{}
	err := m.withProduct(ctx, uid, productID, false, func(tx ManagementTx, _ *entities.Product, data entities.ProductPublication) error {
		if _, err := managedColor(data, colorID); err != nil {
			return err
		}
		var err error
		result, err = tx.Images(ctx, colorID)
		return err
	})
	return result, err
}
func imageIDs(images []entities.ProductImage) []int64 {
	ids := make([]int64, len(images))
	for i, image := range images {
		ids[i] = image.ID()
	}
	return ids
}
func imageIndex(images []entities.ProductImage, id int64) (int, error) {
	for i, image := range images {
		if image.ID() == id {
			return i, nil
		}
	}
	return 0, ErrResourceHidden
}
func imageURLConflict(images []entities.ProductImage, id int64, url string) error {
	for _, image := range images {
		if image.ID() != id && image.URL() == url {
			return ErrConflict
		}
	}
	return nil
}

func (m *Management) CreateImage(ctx context.Context, uid, productID, colorID int64, url string, position *int) (*entities.ProductImage, error) {
	var result *entities.ProductImage
	err := m.withProduct(ctx, uid, productID, true, func(tx ManagementTx, _ *entities.Product, data entities.ProductPublication) error {
		if _, err := managedColor(data, colorID); err != nil {
			return err
		}
		images, err := tx.Images(ctx, colorID)
		if err != nil {
			return err
		}
		if len(images) == 10 {
			return ErrConflict
		}
		pos := len(images)
		if position != nil {
			pos = *position
		}
		if pos < 0 || pos > len(images) {
			return entities.ErrInvalidValue
		}
		image, err := entities.NewProductImage(colorID, url, pos)
		if err != nil {
			return err
		}
		if err = imageURLConflict(images, 0, url); err != nil {
			return err
		}
		image, err = tx.InsertImage(ctx, *image)
		if err != nil {
			return err
		}
		images = slices.Insert(images, pos, *image)
		if err = tx.SaveImagePositions(ctx, colorID, imageIDs(images)); err != nil {
			return err
		}
		result = image
		return nil
	})
	return result, err
}
func (m *Management) ChangeImage(ctx context.Context, uid, productID, colorID, imageID int64, patch ImagePatch) (*entities.ProductImage, error) {
	var result *entities.ProductImage
	err := m.withProduct(ctx, uid, productID, true, func(tx ManagementTx, _ *entities.Product, data entities.ProductPublication) error {
		if _, err := managedColor(data, colorID); err != nil {
			return err
		}
		images, err := tx.Images(ctx, colorID)
		if err != nil {
			return err
		}
		index, err := imageIndex(images, imageID)
		if err != nil {
			return err
		}
		if patch.URL == nil && patch.Position == nil {
			return entities.ErrInvalidValue
		}
		image := images[index]
		if patch.URL != nil {
			if err = image.ChangeURL(*patch.URL); err != nil {
				return err
			}
			if err = imageURLConflict(images, imageID, image.URL()); err != nil {
				return err
			}
		}
		pos := index
		if patch.Position != nil {
			pos = *patch.Position
		}
		if pos < 0 || pos >= len(images) {
			return entities.ErrInvalidValue
		}
		if err = image.ChangePosition(pos); err != nil {
			return err
		}
		images = slices.Delete(images, index, index+1)
		images = slices.Insert(images, pos, image)
		if err = tx.SaveImage(ctx, image); err != nil {
			return err
		}
		if err = tx.SaveImagePositions(ctx, colorID, imageIDs(images)); err != nil {
			return err
		}
		result = &image
		return nil
	})
	return result, err
}
func (m *Management) DeleteImage(ctx context.Context, uid, productID, colorID, imageID int64) error {
	return m.withProduct(ctx, uid, productID, true, func(tx ManagementTx, _ *entities.Product, data entities.ProductPublication) error {
		if _, err := managedColor(data, colorID); err != nil {
			return err
		}
		images, err := tx.Images(ctx, colorID)
		if err != nil {
			return err
		}
		index, err := imageIndex(images, imageID)
		if err != nil {
			return err
		}
		if err = tx.DeleteImage(ctx, colorID, imageID); err != nil {
			return err
		}
		return tx.SaveImagePositions(ctx, colorID, imageIDs(slices.Delete(images, index, index+1)))
	})
}
func (m *Management) ReorderImages(ctx context.Context, uid, productID, colorID int64, ids []int64) ([]entities.ProductImage, error) {
	result := []entities.ProductImage{}
	err := m.withProduct(ctx, uid, productID, true, func(tx ManagementTx, _ *entities.Product, data entities.ProductPublication) error {
		if _, err := managedColor(data, colorID); err != nil {
			return err
		}
		images, err := tx.Images(ctx, colorID)
		if err != nil {
			return err
		}
		if len(ids) != len(images) {
			return entities.ErrInvalidValue
		}
		seen := map[int64]bool{}
		for _, id := range ids {
			if id <= 0 || seen[id] {
				return entities.ErrInvalidValue
			}
			seen[id] = true
			if _, err = imageIndex(images, id); err != nil {
				return err
			}
		}
		if err = tx.SaveImagePositions(ctx, colorID, ids); err != nil {
			return err
		}
		result, err = tx.Images(ctx, colorID)
		return err
	})
	return result, err
}
