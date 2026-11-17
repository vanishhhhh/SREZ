package config

import (
	"errors"
	"net"
	"net/mail"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Mail struct {
	Host, Port, From, Username, Password, TLSMode, FrontendBaseURL string
	IPMinute, IPHour                                               int
}

func LoadMail(defaultOrigin string, localHTTP bool) (Mail, error) {
	value := func(k, d string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		return d
	}
	c := Mail{Host: value("SREZ_SMTP_HOST", "127.0.0.1"), Port: value("SREZ_SMTP_PORT", "1025"), From: value("SREZ_MAIL_FROM", "SREZ <no-reply@srez.local>"), Username: os.Getenv("SREZ_SMTP_USERNAME"), Password: os.Getenv("SREZ_SMTP_PASSWORD"), TLSMode: value("SREZ_SMTP_TLS", "none"), FrontendBaseURL: strings.TrimRight(value("SREZ_FRONTEND_BASE_URL", defaultOrigin), "/"), IPMinute: 10, IPHour: 50}
	invalid := errors.New("invalid mail configuration")
	port, e := strconv.Atoi(c.Port)
	if e != nil || port < 1 || port > 65535 || c.Host == "" || strings.ContainsAny(c.Host, "\r\n\x00 /\\") {
		return c, invalid
	}
	if strings.ContainsAny(c.From, "\r\n\x00") {
		return c, invalid
	}
	from, e := mail.ParseAddress(c.From)
	if e != nil || !strings.Contains(from.Address, "@") {
		return c, invalid
	}
	u, e := url.Parse(c.FrontendBaseURL)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || u.Opaque != "" {
		return c, invalid
	}
	loopback := func(host string) bool {
		if host == "localhost" {
			return true
		}
		ip := net.ParseIP(host)
		return ip != nil && ip.IsLoopback()
	}
	if u.Scheme != "https" && !(localHTTP && u.Scheme == "http" && loopback(u.Hostname())) {
		return c, invalid
	}
	switch c.TLSMode {
	case "none":
		if (c.Host != "mailpit" && !loopback(c.Host)) || c.Username != "" || c.Password != "" {
			return c, invalid
		}
	case "starttls", "tls":
	default:
		return c, invalid
	}
	if (c.Username == "") != (c.Password == "") || strings.ContainsAny(c.Username+c.Password, "\r\n\x00") {
		return c, invalid
	}
	for key, target := range map[string]*int{"SREZ_EMAIL_IP_MINUTE_LIMIT": &c.IPMinute, "SREZ_EMAIL_IP_HOUR_LIMIT": &c.IPHour} {
		if raw := os.Getenv(key); raw != "" {
			n, e := strconv.Atoi(raw)
			if e != nil || n < 1 || n > 10000 {
				return c, invalid
			}
			*target = n
		}
	}
	return c, nil
}
