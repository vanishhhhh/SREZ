package usecase

import (
	"context"
	"errors"
	"time"

	"srez/internal/entities"
)

var ErrEmptyCart = errors.New("cart is empty")
var ErrInventoryConflict = errors.New("inventory update conflict")

type DueOrder struct{ ID, UserID int64 }

type CheckoutTx interface {
	LockCatalog(context.Context) error
	Lines(context.Context) ([]CartLine, error)
	Purchase(context.Context, entities.Order) (*entities.Order, error)
}
type OrderRepository interface {
	WithinCheckout(context.Context, int64, func(CheckoutTx) error) error
	ListOrders(context.Context, int64) ([]entities.Order, error)
	FindOrder(context.Context, int64, int64) (*entities.Order, error)
	WithinOrder(context.Context, int64, int64, func(*entities.Order) error) (*entities.Order, error)
	DueOrders(context.Context, time.Time, int) ([]DueOrder, error)
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

func (o *Orders) Cancel(ctx context.Context, uid, id int64) (*entities.Order, error) {
	if uid <= 0 || id <= 0 {
		return nil, entities.ErrInvalidValue
	}
	return o.repo.WithinOrder(ctx, uid, id, func(order *entities.Order) error {
		return order.Cancel(time.Now().UTC().Truncate(time.Microsecond))
	})
}

// Expiry is a trusted server task, not an HTTP operation taking client time.
// Process orders in separate transactions so one corrupt inventory row does
// not roll back successful releases of other buyers' reservations.
func (o *Orders) Expire(ctx context.Context, now time.Time, limit int) (int, error) {
	if now.IsZero() || limit < 1 || limit > 100 {
		return 0, entities.ErrInvalidValue
	}
	refs, err := o.repo.DueOrders(ctx, now, limit)
	if err != nil {
		return 0, err
	}
	done := 0
	var first error
	for _, ref := range refs {
		if err := ctx.Err(); err != nil {
			return done, err
		}
		changed := false
		_, err := o.repo.WithinOrder(ctx, ref.UserID, ref.ID, func(order *entities.Order) error {
			if order.Status() != "unpaid" || now.Before(order.CancelBefore()) {
				return nil
			}
			if err := order.Expire(now.UTC().Truncate(time.Microsecond)); err != nil {
				return err
			}
			changed = true
			return nil
		})
		if err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		if changed {
			done++
		}
	}
	return done, first
}

// RunReservationExpiry performs a pass immediately and then every interval.
// Each pass is bounded; multiple app instances safely share the same row locks.
func (o *Orders) RunReservationExpiry(ctx context.Context, interval time.Duration, onError func(error)) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		pass, cancel := context.WithTimeout(ctx, 10*time.Second)
		for {
			n, err := o.Expire(pass, time.Now().UTC(), 100)
			if err != nil {
				if onError != nil && ctx.Err() == nil {
					onError(err)
				}
				break
			}
			if n < 100 {
				break
			}
		}
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
