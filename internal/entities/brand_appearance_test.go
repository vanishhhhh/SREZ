package entities_test

import (
	"srez/internal/entities"
	"testing"
)

func TestBrandAppearance(t *testing.T) {
	for _, tt := range []struct {
		value string
		valid bool
	}{
		{"", true}, {"#abc", true}, {"#AbCd", true}, {"#12abEF", true}, {"#12abEF00", true},
		{"red", false}, {"123456", false}, {"#12", false}, {"#12345", false}, {"#gggggg", false}, {" #fff", false},
	} {
		for _, field := range []string{"accent", "background", "text"} {
			t.Run(field+tt.value, func(t *testing.T) {
				var a, b, c string
				switch field {
				case "accent":
					a = tt.value
				case "background":
					b = tt.value
				case "text":
					c = tt.value
				}
				appearance, err := entities.NewBrandAppearance(a, b, c)
				if !tt.valid {
					if err == nil {
						t.Fatal("accepted invalid HEX")
					}
					return
				}
				noError(t, err)
				same(t, appearance, entities.BrandAppearance{AccentColor: a, BackgroundColor: b, TextColor: c})
			})
		}
	}
}
