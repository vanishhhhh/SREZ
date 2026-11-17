package entities

import (
	"strings"
	"testing"
)

func TestWishlistInvariants(t *testing.T) {
	title, desc, err := WishlistText("  Мои хотелки  ", "  Осенняя подборка\nДля себя  ")
	if err != nil || title != "Мои хотелки" || desc != "Осенняя подборка\nДля себя" {
		t.Fatal("normalization")
	}
	for _, v := range []string{"", "  ", strings.Repeat("я", 101), "bad\nname", "bad\x00name", string([]byte{0xff})} {
		if _, _, err = WishlistText(v, ""); err == nil {
			t.Fatal("invalid title accepted")
		}
	}
	for _, v := range []string{strings.Repeat("я", 2001), "bad\x00description", string([]byte{0xff})} {
		if _, _, err = WishlistText("OK", v); err == nil {
			t.Fatal("invalid description accepted")
		}
	}
	if _, _, err = WishlistText(strings.Repeat("я", 100), strings.Repeat("я", 2000)); err != nil {
		t.Fatal("unicode boundary")
	}
	zero, negative, valid := int64(0), int64(-1), int64(9)
	for _, v := range []*int64{&zero, &negative} {
		if ValidateWishlistItem(1, v) == nil {
			t.Fatal("bad variant")
		}
	}
	if ValidateWishlistItem(0, nil) == nil || ValidateWishlistItem(1, nil) != nil || ValidateWishlistItem(1, &valid) != nil {
		t.Fatal("item invariant")
	}
	for _, ids := range [][]int64{{0}, {-1}, {1, 1}, make([]int64, 201)} {
		if ValidateWishlistOrder(ids) == nil {
			t.Fatal("invalid order")
		}
	}
	if ValidateWishlistOrder([]int64{3, 1, 2}) != nil || ValidateWishlistOrder([]int64{}) != nil {
		t.Fatal("valid permutation")
	}
}
