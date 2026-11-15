package entities

import (
	"math"
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
