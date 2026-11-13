package config

import (
	"encoding/base64"
	"errors"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Auth struct {
	JWTKey, CSRFKey          []byte
	Issuer, Audience, Origin string
	LocalHTTP                bool
	Proxies                  []string
	Attempts, HashWorkers    int
}

func LoadAuth() (Auth, error) {
	var c Auth
	bad := errors.New("invalid auth configuration; check keys, origin and limits")
	var err error
	c.JWTKey, err = base64.StdEncoding.DecodeString(os.Getenv("SREZ_JWT_KEY"))
	if err != nil || len(c.JWTKey) < 32 {
		return c, bad
	}
	c.CSRFKey, err = base64.StdEncoding.DecodeString(os.Getenv("SREZ_CSRF_KEY"))
	if err != nil || len(c.CSRFKey) < 32 || string(c.JWTKey) == string(c.CSRFKey) {
		return c, bad
	}
	c.Issuer = os.Getenv("SREZ_JWT_ISSUER")
	c.Audience = os.Getenv("SREZ_JWT_AUDIENCE")
	c.Origin = os.Getenv("SREZ_PUBLIC_ORIGIN")
	u, err := url.Parse(c.Origin)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || u.String() != c.Origin {
		return c, bad
	}
	c.LocalHTTP = os.Getenv("SREZ_AUTH_LOCAL_HTTP") == "true"
	if c.LocalHTTP {
		if u.Scheme != "http" || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" && u.Hostname() != "::1") {
			return c, bad
		}
	} else if u.Scheme != "https" {
		return c, bad
	}
	if c.Issuer == "" || c.Audience == "" {
		return c, bad
	}
	c.Attempts = 10
	c.HashWorkers = 2
	for name, target := range map[string]*int{"SREZ_AUTH_ATTEMPTS": &c.Attempts, "SREZ_AUTH_HASH_WORKERS": &c.HashWorkers} {
		if v := os.Getenv(name); v != "" {
			n, e := strconv.Atoi(v)
			if e != nil || n < 1 || n > 100 {
				return c, bad
			}
			*target = n
		}
	}
	if c.HashWorkers > 8 {
		return c, bad
	}
	if value := os.Getenv("SREZ_TRUSTED_PROXIES"); value != "" {
		c.Proxies = strings.Split(value, ",")
	}
	return c, nil
}
