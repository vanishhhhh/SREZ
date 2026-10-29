package entities_test

import (
	"srez/internal/entities"
	"strings"
	"testing"
	"time"
)

func newBrand(t *testing.T) *entities.Brand {
	t.Helper()
	b, err := entities.NewBrand("Бренд", "brand", created)
	noError(t, err)
	return b
}

func readyBrand(t *testing.T) *entities.Brand {
	t.Helper()
	b := newBrand(t)
	noError(t, b.ChangeDescription("О бренде", created))
	noError(t, b.ChangeLogo("https://example.com/logo", created))
	return b
}

func TestNewBrand(t *testing.T) {
	for _, tt := range []struct {
		name, slug string
		valid      bool
	}{
		{"Бр", "", true}, {strings.Repeat("я", 100), strings.Repeat("a", 60), true}, {"Brand", "a-1", true},
		{"я", "", false}, {strings.Repeat("я", 101), "", false}, {"  ", "", false},
		{"Brand", "ab", false}, {"Brand", strings.Repeat("a", 61), false}, {"Brand", "Brand", false},
		{"Brand", "бренд", false}, {"Brand", "a_b", false},
	} {
		t.Run(tt.name+"/"+tt.slug, func(t *testing.T) {
			b, err := entities.NewBrand(tt.name, tt.slug, created)
			if !tt.valid {
				if err == nil || b != nil {
					t.Fatal("invalid brand accepted")
				}
				return
			}
			noError(t, err)
			same(t, b.ID(), int64(0))
			same(t, b.Name(), tt.name)
			same(t, b.Slug(), tt.slug)
			same(t, b.Status(), entities.BrandDraft)
			same(t, b.CreatedAt(), created)
			same(t, b.UpdatedAt(), created)
		})
	}
	_, err := entities.NewBrand("Brand", "", time.Time{})
	if err == nil {
		t.Fatal("zero time accepted")
	}
}

func TestBrandChanges(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*entities.Brand) error
		valid  bool
	}{
		{"name", func(b *entities.Brand) error { return b.ChangeName("Имя", changed) }, true},
		{"slug", func(b *entities.Brand) error { return b.ChangeSlug("new-brand", changed) }, true},
		{"description max", func(b *entities.Brand) error { return b.ChangeDescription(strings.Repeat("я", 5000), changed) }, true},
		{"logo", func(b *entities.Brand) error { return b.ChangeLogo("https://example.com/l", changed) }, true},
		{"banner", func(b *entities.Brand) error { return b.ChangeBanner("https://example.com/b", changed) }, true},
		{"contacts", func(b *entities.Brand) error {
			return b.ChangeContacts(entities.BrandContacts{Email: "a@example.com"}, changed)
		}, true},
		{"appearance", func(b *entities.Brand) error {
			return b.ChangeAppearance(entities.BrandAppearance{AccentColor: "#123"}, changed)
		}, true},
		{"empty name", func(b *entities.Brand) error { return b.ChangeName("", changed) }, false},
		{"bad slug", func(b *entities.Brand) error { return b.ChangeSlug("ABC", changed) }, false},
		{"description over", func(b *entities.Brand) error { return b.ChangeDescription(strings.Repeat("я", 5001), changed) }, false},
		{"bad logo", func(b *entities.Brand) error { return b.ChangeLogo("http://example.com", changed) }, false},
		{"bad banner", func(b *entities.Brand) error { return b.ChangeBanner("/banner", changed) }, false},
		{"bad contacts", func(b *entities.Brand) error { return b.ChangeContacts(entities.BrandContacts{Email: "bad"}, changed) }, false},
		{"bad appearance", func(b *entities.Brand) error {
			return b.ChangeAppearance(entities.BrandAppearance{TextColor: "red"}, changed)
		}, false},
		{"old time", func(b *entities.Brand) error { return b.ChangeName("Новое", created.Add(-time.Second)) }, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := newBrand(t)
			before := b.State()
			err := tt.change(b)
			if !tt.valid {
				unchanged(t, before, b.State(), err, entities.ErrInvalidValue)
				return
			}
			noError(t, err)
			before.UpdatedAt = changed
			switch tt.name {
			case "name":
				before.Name = "Имя"
			case "slug":
				before.Slug = "new-brand"
			case "description max":
				before.Description = strings.Repeat("я", 5000)
			case "logo":
				before.LogoURL = "https://example.com/l"
			case "banner":
				before.BannerURL = "https://example.com/b"
			case "contacts":
				before.Contacts = entities.BrandContacts{Email: "a@example.com"}
			case "appearance":
				before.Appearance = entities.BrandAppearance{AccentColor: "#123"}
			}
			same(t, b.State(), before)
			s := b.State()
			same(t, b.Name(), s.Name)
			same(t, b.Slug(), s.Slug)
			same(t, b.Description(), s.Description)
			same(t, b.LogoURL(), s.LogoURL)
			same(t, b.BannerURL(), s.BannerURL)
			same(t, b.Contacts(), s.Contacts)
			same(t, b.Appearance(), s.Appearance)
		})
	}
}

func TestBrandTransitions(t *testing.T) {
	for _, from := range []entities.BrandStatus{entities.BrandDraft, entities.BrandActive, entities.BrandSuspended} {
		for _, action := range []string{"publish", "unpublish"} {
			t.Run(string(from)+"/"+action, func(t *testing.T) {
				s := readyBrand(t).State()
				s.ID = 7
				s.Status = from
				b, err := entities.RestoreBrand(s)
				noError(t, err)
				before := b.State()
				valid := (action == "publish" && from == entities.BrandDraft) || (action == "unpublish" && from == entities.BrandActive)
				if action == "publish" {
					err = b.Publish(changed)
				} else {
					err = b.Unpublish(changed)
				}
				if !valid {
					unchanged(t, before, b.State(), err, entities.ErrInvalidTransition)
					return
				}
				noError(t, err)
				same(t, b.UpdatedAt(), changed)
				if action == "publish" {
					same(t, b.Status(), entities.BrandActive)
				} else {
					same(t, b.Status(), entities.BrandDraft)
				}
			})
		}
	}
}

func TestBrandPublicationRequirements(t *testing.T) {
	for _, field := range []string{"slug", "description", "logo"} {
		t.Run(field, func(t *testing.T) {
			b := readyBrand(t)
			switch field {
			case "slug":
				noError(t, b.ChangeSlug("", created))
			case "description":
				noError(t, b.ChangeDescription(" ", created))
			case "logo":
				noError(t, b.ChangeLogo("", created))
			}
			before := b.State()
			err := b.Publish(changed)
			unchanged(t, before, b.State(), err, entities.ErrNotReady)
		})
	}
	for _, field := range []string{"slug", "description", "logo"} {
		t.Run("active/"+field, func(t *testing.T) {
			b := readyBrand(t)
			noError(t, b.Publish(created))
			before := b.State()
			var err error
			switch field {
			case "slug":
				err = b.ChangeSlug("", changed)
			case "description":
				err = b.ChangeDescription("", changed)
			case "logo":
				err = b.ChangeLogo("", changed)
			}
			unchanged(t, before, b.State(), err, entities.ErrNotReady)
		})
	}
}

func TestBrandRestoreAndValues(t *testing.T) {
	s := readyBrand(t).State()
	s.ID = 12
	b, err := entities.RestoreBrand(s)
	noError(t, err)
	same(t, b.ID(), int64(12))
	s.Name = "External"
	same(t, b.Name(), "Бренд")
	c := b.Contacts()
	c.Email = "bad"
	same(t, b.Contacts(), entities.BrandContacts{})
	a := b.Appearance()
	a.TextColor = "red"
	same(t, b.Appearance(), entities.BrandAppearance{})
	for _, mutate := range []func(*entities.BrandState){
		func(s *entities.BrandState) { s.ID = 0 }, func(s *entities.BrandState) { s.Status = "bad" }, func(s *entities.BrandState) { s.Name = "" },
		func(s *entities.BrandState) { s.UpdatedAt = created.Add(-time.Second) },
	} {
		invalid := b.State()
		mutate(&invalid)
		if _, err := entities.RestoreBrand(invalid); err == nil {
			t.Fatal("invalid restored state accepted")
		}
	}
}

func TestBrandTextNormalization(t *testing.T) {
	for _, tt := range []struct{ input, want string }{
		{" \tБренд\u00a0", "Бренд"},
		{" " + strings.Repeat("я", 100) + " ", strings.Repeat("я", 100)},
	} {
		t.Run(tt.input, func(t *testing.T) {
			b, err := entities.NewBrand(tt.input, "brand", created)
			noError(t, err)
			same(t, b.Name(), tt.want)
			s := b.State()
			s.ID = 1
			s.Name = tt.input
			s.Description = " О бренде "
			b, err = entities.RestoreBrand(s)
			noError(t, err)
			same(t, b.Name(), tt.want)
			same(t, b.Description(), "О бренде")
			noError(t, b.ChangeName(" Новое имя ", changed))
			noError(t, b.ChangeDescription(" Новое описание ", changed))
			same(t, b.Name(), "Новое имя")
			same(t, b.Description(), "Новое описание")
			before := b.State()
			err = b.ChangeName(" я ", changed.Add(time.Hour))
			unchanged(t, before, b.State(), err, entities.ErrInvalidValue)
		})
	}
	b := readyBrand(t)
	noError(t, b.Publish(created))
	before := b.State()
	err := b.ChangeDescription(" \t\u00a0", changed)
	unchanged(t, before, b.State(), err, entities.ErrNotReady)
	same(t, b.Status(), entities.BrandActive)
}
