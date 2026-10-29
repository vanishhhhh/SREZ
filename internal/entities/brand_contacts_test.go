package entities_test

import (
	"srez/internal/entities"
	"testing"
)

func TestBrandContacts(t *testing.T) {
	for _, tt := range []struct {
		name, email, telegram, website string
		valid                          bool
	}{
		{"empty", "", "", "", true},
		{"full", "brand@example.com", "https://t.me/brand", "https://example.com", true},
		{"email", "bad", "", "", false},
		{"telegram", "", "http://t.me/brand", "", false},
		{"website", "", "", "ftp://example.com", false},
		{"relative", "", "", "/brand", false},
		{"host", "", "", "https://", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, err := entities.NewBrandContacts(tt.email, tt.telegram, tt.website)
			if !tt.valid {
				if err == nil {
					t.Fatal("accepted invalid contacts")
				}
				return
			}
			noError(t, err)
			same(t, c, entities.BrandContacts{Email: tt.email, TelegramURL: tt.telegram, WebsiteURL: tt.website})
		})
	}
}
