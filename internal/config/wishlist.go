package config

import (
	"encoding/base64"
	"errors"
	"os"
)

// Wishlist link encryption has its own persistent key. Rotating JWT/CSRF keys
// must not make existing owner links unreadable.
func LoadWishlistKey() ([]byte, error) {
	value := os.Getenv("SREZ_WISHLIST_KEY")
	key, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(key) < 32 || value == os.Getenv("SREZ_JWT_KEY") || value == os.Getenv("SREZ_CSRF_KEY") {
		return nil, errors.New("invalid wishlist key configuration")
	}
	return key, nil
}
