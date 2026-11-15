package entities

import (
	"errors"
	"math"
)

const MaxCartQuantity int64 = 100

var ErrMoneyOverflow = errors.New("money exceeds int64 kopecks")

func ValidateCartQuantity(quantity int64) error {
	if quantity < 1 || quantity > MaxCartQuantity {
		return invalid("quantity")
	}
	return nil
}

func LineTotal(unitPrice, quantity int64) (int64, error) {
	if unitPrice < 0 || quantity < 1 {
		return 0, invalid("money")
	}
	if unitPrice > math.MaxInt64/quantity {
		return 0, ErrMoneyOverflow
	}
	return unitPrice * quantity, nil
}

func AddMoney(total, amount int64) (int64, error) {
	if total < 0 || amount < 0 {
		return 0, invalid("money")
	}
	if amount > math.MaxInt64-total {
		return 0, ErrMoneyOverflow
	}
	return total + amount, nil
}
