package config

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestWishlistKeyIsIndependentAndPersistent(t *testing.T) {
	jwt := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	csrf := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32))
	valid := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32))
	t.Setenv("SREZ_JWT_KEY", jwt)
	t.Setenv("SREZ_CSRF_KEY", csrf)
	for _, bad := range []string{"", "invalid", base64.StdEncoding.EncodeToString(make([]byte, 31)), jwt, csrf} {
		t.Setenv("SREZ_WISHLIST_KEY", bad)
		if _, err := LoadWishlistKey(); err == nil {
			t.Fatal("invalid or reused key")
		}
	}
	t.Setenv("SREZ_WISHLIST_KEY", valid)
	key, err := LoadWishlistKey()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SREZ_CSRF_KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{4}, 32)))
	after, err := LoadWishlistKey()
	if err != nil || !bytes.Equal(key, after) {
		t.Fatal("auth key rotation changed wishlist encryption")
	}
}
