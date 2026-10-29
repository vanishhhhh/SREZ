package entities

// BrandAppearance is a value object. Empty colors use the application's defaults.
// CSS HEX forms #RGB, #RGBA, #RRGGBB and #RRGGBBAA are accepted.
type BrandAppearance struct{ AccentColor, BackgroundColor, TextColor string }

func NewBrandAppearance(accent, background, text string) (BrandAppearance, error) {
	a := BrandAppearance{AccentColor: accent, BackgroundColor: background, TextColor: text}
	if err := a.Validate(); err != nil {
		return BrandAppearance{}, err
	}
	return a, nil
}

func (a BrandAppearance) Validate() error {
	for _, c := range []string{a.AccentColor, a.BackgroundColor, a.TextColor} {
		if c != "" && !hexColor.MatchString(c) {
			return invalid("appearance.color")
		}
	}
	return nil
}
