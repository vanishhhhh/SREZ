package security

import (
	"bytes"
	"testing"
)

func TestWishlistLinksConfidentialityAndRevocationMaterial(t *testing.T) {
	if _, err := NewWishlistLinks(make([]byte, 31)); err == nil {
		t.Fatal("short encryption key")
	}
	links, err := NewWishlistLinks(bytes.Repeat([]byte{71}, 32))
	if err != nil {
		t.Fatal(err)
	}
	other, _ := NewWishlistLinks(bytes.Repeat([]byte{72}, 32))
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		token, hash, sealed, err := links.New()
		if err != nil || len(token) != 43 || len(hash) != 32 || !ValidRandom(token) || seen[token] {
			t.Fatal("invalid entropy material")
		}
		seen[token] = true
		if bytes.Contains(sealed, []byte(token)) || !bytes.Equal(hash, Digest(token)) {
			t.Fatal("plaintext or digest mismatch")
		}
		got, err := links.Open(sealed)
		if err != nil || got != token {
			t.Fatal("owner cannot recover link")
		}
		if _, err = other.Open(sealed); err == nil {
			t.Fatal("wrong key accepted")
		}
		sealed[len(sealed)-1] ^= 1
		if _, err = links.Open(sealed); err == nil {
			t.Fatal("tamper accepted")
		}
	}
	for _, v := range []string{"", "123", Random() + "=", Random() + "x", "../private"} {
		if _, err = links.Hash(v); err == nil {
			t.Fatal("invalid capability accepted")
		}
	}
	if _, err = links.Open(nil); err == nil {
		t.Fatal("empty ciphertext accepted")
	}
}
