package entities

// BrandContacts is a value object; empty contacts are allowed.
type BrandContacts struct{ Email, TelegramURL, WebsiteURL string }

func NewBrandContacts(email, telegramURL, websiteURL string) (BrandContacts, error) {
	c := BrandContacts{Email: email, TelegramURL: telegramURL, WebsiteURL: websiteURL}
	if err := c.Validate(); err != nil {
		return BrandContacts{}, err
	}
	return c, nil
}

func (c BrandContacts) Validate() error {
	if c.Email != "" && !validEmail(c.Email) {
		return invalid("contacts.email")
	}
	if !validURL(c.TelegramURL, true) {
		return invalid("contacts.telegram_url")
	}
	if !validURL(c.WebsiteURL, true) {
		return invalid("contacts.website_url")
	}
	return nil
}
