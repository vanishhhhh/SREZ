package usecase

import (
	"context"
	"errors"

	"srez/internal/entities"
)

var (
	ErrCartUnavailable   = errors.New("cart contains unavailable items")
	ErrInsufficientStock = errors.New("insufficient stock")
)

type Favorite struct {
	ProductID int64
	Product   *ProductCard
}

type CartProduct struct {
	ProductID, BrandID, VariantID      int64
	Name, Color, Size, PreviewImageURL string
	UnitPriceKopecks, Stock            int64
}

type CartLine struct {
	ID              int64
	VariantID       *int64
	Quantity        int64
	Product         *CartProduct
	Availability    string
	SubtotalKopecks int64
}

type Cart struct {
	Items        []CartLine
	TotalKopecks int64
}

type ShoppingRepository interface {
	Favorites(context.Context, int64) ([]Favorite, error)
	AddFavorite(context.Context, int64, int64) error
	RemoveFavorite(context.Context, int64, int64) error
	Cart(context.Context, int64) ([]CartLine, error)
	WithinCart(context.Context, int64, func(CartTx) error) error
}

// WithinCart serializes buyer mutations. LockVariant follows the catalog's
// brand -> product -> children order before any cart row is changed.
type CartTx interface {
	Item(context.Context, int64) (*CartLine, error)
	LockVariant(context.Context, int64) (*CartProduct, error)
	VariantQuantity(context.Context, int64) (int64, error)
	Add(context.Context, int64, int64) error
	Set(context.Context, int64, int64) error
	Remove(context.Context, int64) error
	Lines(context.Context) ([]CartLine, error)
}

type Shopping struct{ repo ShoppingRepository }

func NewShopping(repo ShoppingRepository) *Shopping { return &Shopping{repo} }
func buyerID(uid int64) error {
	if uid <= 0 {
		return entities.ErrInvalidValue
	}
	return nil
}
func (s *Shopping) Favorites(ctx context.Context, uid int64) ([]Favorite, error) {
	if err := buyerID(uid); err != nil {
		return nil, err
	}
	return s.repo.Favorites(ctx, uid)
}
func (s *Shopping) AddFavorite(ctx context.Context, uid, product int64) error {
	if uid <= 0 || product <= 0 {
		return entities.ErrInvalidValue
	}
	return s.repo.AddFavorite(ctx, uid, product)
}
func (s *Shopping) RemoveFavorite(ctx context.Context, uid, product int64) error {
	if uid <= 0 || product <= 0 {
		return entities.ErrInvalidValue
	}
	return s.repo.RemoveFavorite(ctx, uid, product)
}

// Only currently public lines with sufficient stock contribute to the total.
// Hidden/deleted catalog data is absent from Product, including its price.
func CalculateCart(lines []CartLine) (*Cart, error) {
	cart := &Cart{Items: make([]CartLine, 0, len(lines))}
	for _, line := range lines {
		if err := entities.ValidateCartQuantity(line.Quantity); err != nil {
			return nil, err
		}
		line.SubtotalKopecks = 0
		line.Availability = "unavailable"
		if line.Product != nil && line.VariantID != nil {
			line.Availability = "insufficient_stock"
			if line.Quantity <= line.Product.Stock {
				line.Availability = "available"
				subtotal, err := entities.LineTotal(line.Product.UnitPriceKopecks, line.Quantity)
				if err != nil {
					return nil, err
				}
				line.SubtotalKopecks = subtotal
				cart.TotalKopecks, err = entities.AddMoney(cart.TotalKopecks, subtotal)
				if err != nil {
					return nil, err
				}
			}
		}
		cart.Items = append(cart.Items, line)
	}
	return cart, nil
}
func (s *Shopping) Cart(ctx context.Context, uid int64) (*Cart, error) {
	if err := buyerID(uid); err != nil {
		return nil, err
	}
	lines, err := s.repo.Cart(ctx, uid)
	if err != nil {
		return nil, err
	}
	return CalculateCart(lines)
}
func (s *Shopping) ChangeCart(ctx context.Context, uid, id, quantity int64, add bool) (*Cart, error) {
	if uid <= 0 || id <= 0 {
		return nil, entities.ErrInvalidValue
	}
	if err := entities.ValidateCartQuantity(quantity); err != nil {
		return nil, err
	}
	var cart *Cart
	err := s.repo.WithinCart(ctx, uid, func(tx CartTx) error {
		variantID := id
		if !add {
			line, err := tx.Item(ctx, id)
			if err != nil {
				return err
			}
			if line == nil {
				return ErrResourceHidden
			}
			if line.VariantID == nil {
				return ErrCartUnavailable
			}
			variantID = *line.VariantID
		}
		product, err := tx.LockVariant(ctx, variantID)
		if err != nil {
			return err
		}
		if product == nil {
			if add {
				return ErrResourceHidden
			}
			return ErrCartUnavailable
		}
		resulting := quantity
		if add {
			current, err := tx.VariantQuantity(ctx, variantID)
			if err != nil {
				return err
			}
			// Both operands are bounded by MaxCartQuantity.
			resulting += current
		}
		if err = entities.ValidateCartQuantity(resulting); err != nil {
			return err
		}
		if resulting > product.Stock {
			return ErrInsufficientStock
		}
		if add {
			err = tx.Add(ctx, variantID, quantity)
		} else {
			err = tx.Set(ctx, id, quantity)
		}
		if err != nil {
			return err
		}
		lines, err := tx.Lines(ctx)
		if err != nil {
			return err
		}
		cart, err = CalculateCart(lines)
		return err
	})
	return cart, err
}
func (s *Shopping) RemoveCartItem(ctx context.Context, uid, id int64) error {
	if uid <= 0 || id <= 0 {
		return entities.ErrInvalidValue
	}
	return s.repo.WithinCart(ctx, uid, func(tx CartTx) error { return tx.Remove(ctx, id) })
}
