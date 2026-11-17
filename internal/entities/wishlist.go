package entities

import (
	"strings"
	"time"
	"unicode"
)

const (
	MaxWishlists     = 20
	MaxWishlistItems = 200
)

type Wishlist struct {
	ID, OwnerUserID                  int64
	Title, Description, Visibility   string
	ShareTokenHash, ShareTokenSealed []byte
	CreatedAt, UpdatedAt             time.Time
	ItemCount                        int
}

func WishlistText(title, description string) (string, string, error) {
	title, description = strings.TrimSpace(title), strings.TrimSpace(description)
	if !validText(title, 1, 100) || !validText(description, 0, 2000) {
		return "", "", ErrInvalidValue
	}
	for _, r := range title {
		if unicode.IsControl(r) {
			return "", "", ErrInvalidValue
		}
	}
	for _, r := range description {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return "", "", ErrInvalidValue
		}
	}
	return title, description, nil
}

// Each list has one entry per product. The optional variant is a preference,
// not a separate duplicate entry or a reservation of stock.
func ValidateWishlistItem(productID int64, variantID *int64) error {
	if productID <= 0 || (variantID != nil && *variantID <= 0) {
		return ErrInvalidValue
	}
	return nil
}

func ValidateWishlistOrder(ids []int64) error {
	if len(ids) > MaxWishlistItems {
		return ErrInvalidValue
	}
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return ErrInvalidValue
		}
		seen[id] = true
	}
	return nil
}
