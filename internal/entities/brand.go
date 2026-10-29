package entities

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

type BrandStatus string

const (
	BrandDraft     BrandStatus = "draft"
	BrandActive    BrandStatus = "active"
	BrandSuspended BrandStatus = "suspended"
)

type BrandState struct {
	ID                                          int64
	Name, Slug, Description, LogoURL, BannerURL string
	Contacts                                    BrandContacts
	Appearance                                  BrandAppearance
	Status                                      BrandStatus
	CreatedAt, UpdatedAt                        time.Time
}

type Brand struct{ state BrandState }

var slugPattern = regexp.MustCompile(`^[a-z0-9-]{3,60}$`)

func NewBrand(name, slug string, now time.Time) (*Brand, error) {
	s := BrandState{Name: strings.TrimSpace(name), Slug: slug, Status: BrandDraft, CreatedAt: now, UpdatedAt: now}
	if err := validateBrand(s); err != nil {
		return nil, err
	}
	return &Brand{state: s}, nil
}

func RestoreBrand(s BrandState) (*Brand, error) {
	if s.ID <= 0 {
		return nil, invalid("brand.id")
	}
	s.Name = strings.TrimSpace(s.Name)
	s.Description = strings.TrimSpace(s.Description)
	if err := validateBrand(s); err != nil {
		return nil, err
	}
	return &Brand{state: s}, nil
}

func validateBrand(s BrandState) error {
	if s.ID < 0 {
		return invalid("brand.id")
	}
	if !validText(s.Name, 2, 100) {
		return invalid("brand.name")
	}
	if s.Slug != "" && !slugPattern.MatchString(s.Slug) {
		return invalid("brand.slug")
	}
	if !validText(s.Description, 0, 5000) {
		return invalid("brand.description")
	}
	if !validURL(s.LogoURL, true) || !validURL(s.BannerURL, true) {
		return invalid("brand.image_url")
	}
	if err := s.Contacts.Validate(); err != nil {
		return err
	}
	if err := s.Appearance.Validate(); err != nil {
		return err
	}
	if !validTimes(s.CreatedAt, s.UpdatedAt) {
		return invalid("brand.timestamps")
	}
	switch s.Status {
	case BrandDraft, BrandSuspended:
	case BrandActive:
		if s.Slug == "" || strings.TrimSpace(s.Description) == "" || s.LogoURL == "" {
			return fmt.Errorf("brand requires slug, description and logo: %w", ErrNotReady)
		}
	default:
		return invalid("brand.status")
	}
	return nil
}

func (b *Brand) change(s BrandState, now time.Time) error {
	if err := checkTime(now, b.state.UpdatedAt); err != nil {
		return err
	}
	s.Name = strings.TrimSpace(s.Name)
	s.Description = strings.TrimSpace(s.Description)
	s.UpdatedAt = now
	if err := validateBrand(s); err != nil {
		return err
	}
	b.state = s
	return nil
}

func (b *Brand) ChangeName(v string, now time.Time) error {
	s := b.state
	s.Name = v
	return b.change(s, now)
}
func (b *Brand) ChangeSlug(v string, now time.Time) error {
	s := b.state
	s.Slug = v
	return b.change(s, now)
}
func (b *Brand) ChangeDescription(v string, now time.Time) error {
	s := b.state
	s.Description = v
	return b.change(s, now)
}
func (b *Brand) ChangeLogo(v string, now time.Time) error {
	s := b.state
	s.LogoURL = v
	return b.change(s, now)
}
func (b *Brand) ChangeBanner(v string, now time.Time) error {
	s := b.state
	s.BannerURL = v
	return b.change(s, now)
}
func (b *Brand) ChangeContacts(v BrandContacts, now time.Time) error {
	s := b.state
	s.Contacts = v
	return b.change(s, now)
}
func (b *Brand) ChangeAppearance(v BrandAppearance, now time.Time) error {
	s := b.state
	s.Appearance = v
	return b.change(s, now)
}

func (b *Brand) Publish(now time.Time) error {
	if b.state.Status != BrandDraft {
		return ErrInvalidTransition
	}
	s := b.state
	s.Status = BrandActive
	return b.change(s, now)
}

func (b *Brand) Unpublish(now time.Time) error {
	if b.state.Status != BrandActive {
		return ErrInvalidTransition
	}
	s := b.state
	s.Status = BrandDraft
	return b.change(s, now)
}

func (b Brand) State() BrandState           { return b.state }
func (b Brand) ID() int64                   { return b.state.ID }
func (b Brand) Name() string                { return b.state.Name }
func (b Brand) Slug() string                { return b.state.Slug }
func (b Brand) Description() string         { return b.state.Description }
func (b Brand) LogoURL() string             { return b.state.LogoURL }
func (b Brand) BannerURL() string           { return b.state.BannerURL }
func (b Brand) Contacts() BrandContacts     { return b.state.Contacts }
func (b Brand) Appearance() BrandAppearance { return b.state.Appearance }
func (b Brand) Status() BrandStatus         { return b.state.Status }
func (b Brand) CreatedAt() time.Time        { return b.state.CreatedAt }
func (b Brand) UpdatedAt() time.Time        { return b.state.UpdatedAt }
