package usecase

import (
	"context"
	"errors"
	"time"

	"srez/internal/entities"
)

var ErrEmptyCart = errors.New("cart is empty")

type CheckoutTx interface {
	LockCatalog(context.Context) error
	Lines(context.Context) ([]CartLine, error)
	Purchase(context.Context, entities.Order) (*entities.Order, error)
}
type OrderRepository interface {
	WithinCheckout(context.Context, int64, func(CheckoutTx) error) error
	ListOrders(context.Context, int64) ([]entities.Order, error)
	FindOrder(context.Context, int64, int64) (*entities.Order, error)
}
type Orders struct{ repo OrderRepository }

func NewOrders(repo OrderRepository) *Orders { return &Orders{repo} }
func (o *Orders) Checkout(ctx context.Context, uid int64) (*entities.Order, error) {
	if err := buyerID(uid); err != nil {
		return nil, err
	}
	var saved *entities.Order
	err := o.repo.WithinCheckout(ctx, uid, func(tx CheckoutTx) error {
		if err := tx.LockCatalog(ctx); err != nil {
			return err
		}
		lines, err := tx.Lines(ctx)
		if err != nil {
			return err
		}
		if len(lines) == 0 {
			return ErrEmptyCart
		}
		cart, err := CalculateCart(lines)
		if err != nil {
			return err
		}
		snapshots := make([]entities.OrderLineState, 0, len(cart.Items))
		for _, line := range cart.Items {
			switch line.Availability {
			case "unavailable":
				return ErrCartUnavailable
			case "insufficient_stock":
				return ErrInsufficientStock
			}
			p := line.Product
			snapshots = append(snapshots, entities.OrderLineState{ProductVariantID: p.VariantID, ProductID: p.ProductID, BrandID: p.BrandID, ProductName: p.Name, Color: p.Color, Size: p.Size, Quantity: line.Quantity, UnitPriceKopecks: p.UnitPriceKopecks})
		}
		order, err := entities.NewOrder(uid, snapshots, time.Now().UTC())
		if err != nil {
			return err
		}
		saved, err = tx.Purchase(ctx, *order)
		return err
	})
	if err != nil {
		return nil, err
	}
	return saved, nil
}
func (o *Orders) List(ctx context.Context, uid int64) ([]entities.Order, error) {
	if err := buyerID(uid); err != nil {
		return nil, err
	}
	return o.repo.ListOrders(ctx, uid)
}
func (o *Orders) Get(ctx context.Context, uid, id int64) (*entities.Order, error) {
	if uid <= 0 || id <= 0 {
		return nil, entities.ErrInvalidValue
	}
	order, err := o.repo.FindOrder(ctx, uid, id)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, ErrResourceHidden
	}
	return order, nil
}
