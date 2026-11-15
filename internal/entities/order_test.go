package entities

import (
	"errors"
	"math"
	"reflect"
	"testing"
	"time"
)

func TestOrderSnapshotInvariants(t *testing.T) {
	line := OrderLineState{ProductVariantID: 1, ProductID: 2, BrandID: 3, ProductName: "Hoodie", Color: "Black", Size: "M", Quantity: 2, UnitPriceKopecks: 100}
	if _, err := NewOrder(7, []OrderLineState{line, line}, time.Now()); err == nil {
		t.Fatal("accepted duplicate order variant")
	}
	input := []OrderLineState{line}
	now := time.Now().UTC()
	order, err := NewOrder(7, input, now)
	if err != nil {
		t.Fatal(err)
	}
	input[0].ProductName = "changed"
	state := order.State()
	state.Lines[0].ProductName = "mutated"
	if order.Lines()[0].ProductName != "Hoodie" || order.TotalKopecks() != 200 || order.Status() != "unpaid" {
		t.Fatal("mutable snapshot")
	}
	state = order.State()
	state.ID = 1
	if _, err = RestoreOrder(state); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*OrderState){
		func(s *OrderState) { s.TotalKopecks++ }, func(s *OrderState) { s.Status = "paid" },
		func(s *OrderState) { s.Lines[0].Quantity = 0 }, func(s *OrderState) { s.Lines[0].UnitPriceKopecks = math.MaxInt64 },
		func(s *OrderState) { s.Lines = nil }, func(s *OrderState) { s.UserID = 0 }, func(s *OrderState) { s.Lines[0].BrandID = 0 },
	} {
		s := order.State()
		s.ID = 1
		change(&s)
		if _, err = RestoreOrder(s); err == nil {
			t.Fatal("accepted invalid order", s)
		}
	}
}

func TestOrderCancellationWindowAndImmutableSnapshots(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	line := OrderLineState{ProductVariantID: 1, ProductID: 2, BrandID: 3, ProductName: "Hoodie", Color: "Black", Size: "M", Quantity: 2, UnitPriceKopecks: 100}
	order, err := NewOrder(7, []OrderLineState{line}, now)
	if err != nil {
		t.Fatal(err)
	}
	before := order.State()
	if err = order.Cancel(now.Add(OrderCancellationWindow)); !errors.Is(err, ErrOrderCancellationClosed) {
		t.Fatal("deadline must be exclusive", err)
	}
	if !reflect.DeepEqual(before, order.State()) {
		t.Fatal("denied cancellation mutated order")
	}
	if err = order.Cancel(now.Add(-time.Second)); !errors.Is(err, ErrInvalidValue) {
		t.Fatal(err)
	}
	if err = order.Cancel(now.Add(OrderCancellationWindow - time.Microsecond)); err != nil {
		t.Fatal(err)
	}
	if order.Status() != "cancelled" || order.TotalKopecks() != 200 || !reflect.DeepEqual(order.Lines(), before.Lines) {
		t.Fatal("cancellation changed snapshot")
	}
	after := order.State()
	after.ID = 1
	restored, err := RestoreOrder(after)
	if err != nil {
		t.Fatal(err)
	}
	if err = restored.Cancel(now.Add(time.Hour)); err != nil {
		t.Fatal("repeat cancellation must be idempotent", err)
	}
	if !reflect.DeepEqual(after, restored.State()) {
		t.Fatal("repeat cancellation changed timestamp")
	}
	*after.CancelledAt = now.Add(time.Hour)
	if restored.CancelledAt().Equal(*after.CancelledAt) {
		t.Fatal("mutable cancellation timestamp")
	}
	for _, status := range []string{"processing", "paid"} {
		copy := *order
		copy.state.Status = status
		if err = copy.Cancel(now.Add(time.Second)); !errors.Is(err, ErrInvalidTransition) {
			t.Fatal("non-unpaid cancellation accepted", status, err)
		}
	}
}

func TestOrderExpiryBoundaryAndSnapshot(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	line := OrderLineState{ProductVariantID: 1, ProductID: 2, BrandID: 3, ProductName: "Hoodie", Color: "Black", Size: "M", Quantity: 1, UnitPriceKopecks: 100}
	o, err := NewOrder(1, []OrderLineState{line}, now)
	if err != nil {
		t.Fatal(err)
	}
	before := o.State()
	if err = o.Expire(o.CancelBefore().Add(-time.Microsecond)); !errors.Is(err, ErrInvalidTransition) {
		t.Fatal("early expiry accepted", err)
	}
	if !reflect.DeepEqual(before, o.State()) {
		t.Fatal("early expiry changed order")
	}
	if err = o.Expire(o.CancelBefore()); err != nil {
		t.Fatal(err)
	}
	if o.Status() != "expired" || o.ExpiredAt() == nil || o.TotalKopecks() != 100 || !reflect.DeepEqual(o.Lines(), before.Lines) {
		t.Fatal("expiry mutated snapshots")
	}
	if err = o.Cancel(o.CancelBefore()); !errors.Is(err, ErrOrderCancellationClosed) {
		t.Fatal(err)
	}
	s := o.State()
	s.ID = 1
	if _, err = RestoreOrder(s); err != nil {
		t.Fatal(err)
	}
	*s.ExpiredAt = now
	if _, err = RestoreOrder(s); err == nil {
		t.Fatal("invalid expiry timestamp accepted")
	}
}
