package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
)

// Only a SHA-256 digest is used for public lookup. AES-GCM makes an existing
// link recoverable for its owner without storing a plaintext capability token.
// The dedicated persistent key is purpose-separated using HMAC-SHA256.
type WishlistLinks struct{ aead cipher.AEAD }

func NewWishlistLinks(secret []byte) (*WishlistLinks, error) {
	if len(secret) < 32 {
		return nil, ErrInvalid
	}
	h := hmac.New(sha256.New, secret)
	h.Write([]byte("srez/wishlist/share-encryption/v1"))
	block, err := aes.NewCipher(h.Sum(nil))
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &WishlistLinks{aead}, nil
}
func (s *WishlistLinks) New() (string, []byte, []byte, error) {
	token := Random()
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", nil, nil, err
	}
	sealed := s.aead.Seal(nonce, nonce, []byte(token), []byte("srez-wishlist-v1"))
	return token, Digest(token), sealed, nil
}
func (s *WishlistLinks) Open(sealed []byte) (string, error) {
	n := s.aead.NonceSize()
	if len(sealed) < n+s.aead.Overhead() {
		return "", ErrInvalid
	}
	value, err := s.aead.Open(nil, sealed[:n], sealed[n:], []byte("srez-wishlist-v1"))
	if err != nil || !ValidRandom(string(value)) {
		return "", ErrInvalid
	}
	return string(value), nil
}
func (s *WishlistLinks) Hash(token string) ([]byte, error) {
	if !ValidRandom(token) {
		return nil, ErrInvalid
	}
	return Digest(token), nil
}
