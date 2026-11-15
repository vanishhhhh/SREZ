package entities

import "time"

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
	Lines        []OrderLineState
}

// Order is an immutable purchase snapshot. There are no buyer setters for status,
// total or historical lines; later payment/fulfillment need explicit transitions.
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
	return &Order{OrderState{UserID: uid, Status: "unpaid", TotalKopecks: total, CreatedAt: now, Lines: append([]OrderLineState(nil), lines...)}}, nil
}
func RestoreOrder(s OrderState) (*Order, error) {
	if s.ID <= 0 || s.Status != "unpaid" {
		return nil, invalid("order state")
	}
	order, err := NewOrder(s.UserID, s.Lines, s.CreatedAt)
	if err != nil {
		return nil, err
	}
	if s.TotalKopecks != order.TotalKopecks() {
		return nil, invalid("order total")
	}
	order.state.ID = s.ID
	return order, nil
}
func (o Order) ID() int64               { return o.state.ID }
func (o Order) UserID() int64           { return o.state.UserID }
func (o Order) Status() string          { return o.state.Status }
func (o Order) TotalKopecks() int64     { return o.state.TotalKopecks }
func (o Order) CreatedAt() time.Time    { return o.state.CreatedAt }
func (o Order) Lines() []OrderLineState { return append([]OrderLineState(nil), o.state.Lines...) }
func (o Order) State() OrderState       { s := o.state; s.Lines = o.Lines(); return s }
