package entities

import (
	"errors"
	"time"
)

const OrderCancellationWindow = 15 * time.Minute

var ErrOrderCancellationClosed = errors.New("order cancellation window closed")

type OrderLineState struct {
	ProductVariantID, ProductID, BrandID int64
	ProductName, Color, Size             string
	Quantity, UnitPriceKopecks           int64
}

type OrderState struct {
	ID, UserID   int64
	Status       string
	TotalKopecks int64
	CreatedAt    time.Time
	CancelBefore time.Time
	CancelledAt  *time.Time
	ExpiredAt    *time.Time
	Lines        []OrderLineState
}

// Order keeps immutable line and money snapshots. The only buyer transition
// is unpaid -> cancelled within the self-service window; payment is not implemented.
type Order struct{ state OrderState }

func orderTotal(lines []OrderLineState) (int64, error) {
	if len(lines) == 0 {
		return 0, invalid("order lines")
	}
	total := int64(0)
	seen := make(map[int64]bool, len(lines))
	for _, line := range lines {
		if seen[line.ProductVariantID] {
			return 0, invalid("duplicate order variant")
		}
		seen[line.ProductVariantID] = true
		if line.ProductVariantID <= 0 || line.ProductID <= 0 || line.BrandID <= 0 || !validText(line.ProductName, 2, 120) || !nonblankText(line.Color) || !validText(line.Size, 1, 20) {
			return 0, invalid("order snapshot")
		}
		if err := ValidateCartQuantity(line.Quantity); err != nil {
			return 0, err
		}
		if line.UnitPriceKopecks <= 0 {
			return 0, invalid("unit price")
		}
		amount, err := LineTotal(line.UnitPriceKopecks, line.Quantity)
		if err != nil {
			return 0, err
		}
		total, err = AddMoney(total, amount)
		if err != nil {
			return 0, err
		}
	}
	return total, nil
}
func NewOrder(uid int64, lines []OrderLineState, now time.Time) (*Order, error) {
	if uid <= 0 || now.IsZero() {
		return nil, invalid("order")
	}
	total, err := orderTotal(lines)
	if err != nil {
		return nil, err
	}
	return &Order{OrderState{UserID: uid, Status: "unpaid", TotalKopecks: total, CreatedAt: now, CancelBefore: now.Add(OrderCancellationWindow), Lines: append([]OrderLineState(nil), lines...)}}, nil
}
func RestoreOrder(s OrderState) (*Order, error) {
	if s.ID <= 0 || (s.Status != "unpaid" && s.Status != "cancelled" && s.Status != "expired") {
		return nil, invalid("order state")
	}
	order, err := NewOrder(s.UserID, s.Lines, s.CreatedAt)
	if err != nil {
		return nil, err
	}
	if s.TotalKopecks != order.TotalKopecks() || !s.CancelBefore.Equal(order.state.CancelBefore) {
		return nil, invalid("order total")
	}
	switch s.Status {
	case "unpaid":
		if s.CancelledAt != nil || s.ExpiredAt != nil {
			return nil, invalid("cancellation")
		}
	case "cancelled":
		if s.CancelledAt == nil || s.CancelledAt.Before(s.CreatedAt) || !s.CancelledAt.Before(s.CancelBefore) || s.ExpiredAt != nil {
			return nil, invalid("cancellation")
		}
	case "expired":
		if s.ExpiredAt == nil || s.ExpiredAt.Before(s.CancelBefore) || s.CancelledAt != nil {
			return nil, invalid("expiry")
		}
	}

	order.state.ID = s.ID
	order.state.Status = s.Status
	order.state.CancelledAt = copyTime(s.CancelledAt)
	order.state.ExpiredAt = copyTime(s.ExpiredAt)
	return order, nil
}
func (o Order) ID() int64               { return o.state.ID }
func (o Order) UserID() int64           { return o.state.UserID }
func (o Order) Status() string          { return o.state.Status }
func (o Order) TotalKopecks() int64     { return o.state.TotalKopecks }
func (o Order) CreatedAt() time.Time    { return o.state.CreatedAt }
func (o Order) Lines() []OrderLineState { return append([]OrderLineState(nil), o.state.Lines...) }
func (o Order) CancelBefore() time.Time { return o.state.CancelBefore }
func (o Order) CancelledAt() *time.Time { return copyTime(o.state.CancelledAt) }
func (o Order) ExpiredAt() *time.Time   { return copyTime(o.state.ExpiredAt) }
func (o Order) State() OrderState {
	s := o.state
	s.Lines = o.Lines()
	s.CancelledAt = o.CancelledAt()
	s.ExpiredAt = o.ExpiredAt()
	return s
}
func (o *Order) Cancel(now time.Time) error {
	if o.state.Status == "cancelled" {
		return nil
	}
	if o.state.Status == "expired" {
		return ErrOrderCancellationClosed
	}
	if o.state.Status != "unpaid" {
		return ErrInvalidTransition
	}
	if now.IsZero() || now.Before(o.state.CreatedAt) {
		return invalid("time")
	}
	if !now.Before(o.state.CancelBefore) {
		return ErrOrderCancellationClosed
	}
	o.state.Status = "cancelled"
	o.state.CancelledAt = copyTime(&now)
	return nil
}

func (o *Order) Expire(now time.Time) error {
	if o.state.Status == "cancelled" || o.state.Status == "expired" {
		return nil
	}
	if o.state.Status != "unpaid" || now.Before(o.state.CancelBefore) {
		return ErrInvalidTransition
	}
	o.state.Status = "expired"
	o.state.ExpiredAt = copyTime(&now)
	return nil
}
