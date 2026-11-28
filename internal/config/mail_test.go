package config

import "testing"

func TestMailConfigurationFailsClosed(t *testing.T) {
	keys := []string{"SREZ_SMTP_HOST", "SREZ_SMTP_PORT", "SREZ_MAIL_FROM", "SREZ_FRONTEND_BASE_URL", "SREZ_SMTP_TLS", "SREZ_SMTP_USERNAME", "SREZ_SMTP_PASSWORD", "SREZ_EMAIL_IP_MINUTE_LIMIT", "SREZ_EMAIL_IP_HOUR_LIMIT"}
	for _, k := range keys {
		t.Setenv(k, "")
	}
	if _, e := LoadMail("http://localhost:5174", true); e != nil {
		t.Fatal(e)
	}
	for _, pair := range [][2]string{{"SREZ_SMTP_HOST", "smtp.example.com"}, {"SREZ_SMTP_PORT", "0"}, {"SREZ_MAIL_FROM", "x@example.com\r\nBcc:evil@example.com"}, {"SREZ_FRONTEND_BASE_URL", "http://evil.example"}, {"SREZ_FRONTEND_BASE_URL", "https://user:pass@example.com"}, {"SREZ_FRONTEND_BASE_URL", "https://example.com/?token=x"}, {"SREZ_SMTP_TLS", "skipverify"}, {"SREZ_EMAIL_IP_HOUR_LIMIT", "0"}, {"SREZ_SMTP_USERNAME", "secret"}} {
		t.Run(pair[0]+"/invalid", func(t *testing.T) {
			t.Setenv(pair[0], pair[1])
			if _, e := LoadMail("http://localhost:5174", true); e == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
	t.Setenv("SREZ_SMTP_HOST", "smtp.example.com")
	t.Setenv("SREZ_SMTP_TLS", "starttls")
	t.Setenv("SREZ_FRONTEND_BASE_URL", "https://srez.example")
	if _, e := LoadMail("https://srez.example", false); e != nil {
		t.Fatal(e)
	}
	if _, e := LoadMail("https://other.example", false); e == nil {
		t.Fatal("accepted a mail link origin different from the production origin")
	}
	t.Setenv("SREZ_SMTP_HOST", "mailpit")
	t.Setenv("SREZ_SMTP_TLS", "none")
	if _, e := LoadMail("https://srez.example", false); e == nil {
		t.Fatal("accepted cleartext SMTP in production mode")
	}
}
