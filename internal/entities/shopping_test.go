package entities

import (
	"errors"
	"math"
	"testing"
)

func TestShoppingMoneyAndQuantity(t *testing.T) {
	for _, q := range []int64{-1, 0, 101, math.MaxInt64} {
		if ValidateCartQuantity(q) == nil {
			t.Fatalf("accepted %d", q)
		}
	}
	for _, q := range []int64{1, 100} {
		if err := ValidateCartQuantity(q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := LineTotal(math.MaxInt64, 2); !errors.Is(err, ErrMoneyOverflow) {
		t.Fatal(err)
	}
	if _, err := AddMoney(math.MaxInt64, 1); !errors.Is(err, ErrMoneyOverflow) {
		t.Fatal(err)
	}
	if v, err := LineTotal(250001, 3); err != nil || v != 750003 {
		t.Fatal(v, err)
	}
}
