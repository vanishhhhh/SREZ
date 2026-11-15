package usecase

import (
	"errors"
	"math"
	"srez/internal/entities"
	"testing"
)

func TestCartAvailabilityAndTotals(t *testing.T) {
	id := int64(3)
	lines := []CartLine{
		{ID: 1, VariantID: &id, Quantity: 2, Product: &CartProduct{UnitPriceKopecks: 123, Stock: 2}},
		{ID: 2, VariantID: &id, Quantity: 3, Product: &CartProduct{UnitPriceKopecks: 200, Stock: 2}},
		{ID: 3, VariantID: &id, Quantity: 1},
		{ID: 4, Quantity: 1},
	}
	cart, err := CalculateCart(lines)
	if err != nil || cart.TotalKopecks != 246 || len(cart.Items) != 4 {
		t.Fatal(cart, err)
	}
	for i, want := range []string{"available", "insufficient_stock", "unavailable", "unavailable"} {
		if cart.Items[i].Availability != want {
			t.Fatal(cart.Items[i])
		}
		if i > 0 && cart.Items[i].SubtotalKopecks != 0 {
			t.Fatal("unavailable line counted")
		}
	}
	lines[0].Product.UnitPriceKopecks = math.MaxInt64
	if _, err = CalculateCart(lines); !errors.Is(err, entities.ErrMoneyOverflow) {
		t.Fatal(err)
	}
}
