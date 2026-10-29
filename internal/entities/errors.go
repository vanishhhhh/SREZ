// Package entities contains marketplace state and local domain invariants.
// Constructors create transient objects (ID == 0); Restore functions accept
// persisted state. Neither path checks permissions or database uniqueness.
package entities

import (
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrInvalidValue      = errors.New("invalid value")
	ErrInvalidTransition = errors.New("invalid state transition")
	ErrNotReady          = errors.New("not ready for publication")
)

func invalid(field string) error { return fmt.Errorf("%s: %w", field, ErrInvalidValue) }

func validText(s string, min, max int) bool {
	n := utf8.RuneCountInString(s)
	return utf8.ValidString(s) && n >= min && n <= max && (min == 0 || strings.TrimSpace(s) != "")
}

func nonblankText(s string) bool {
	return utf8.ValidString(s) && strings.TrimSpace(s) != ""
}

// normalizedTextKey is for color and size uniqueness, not display formatting.
func normalizedTextKey(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func validEmail(s string) bool {
	a, err := mail.ParseAddress(s)
	return err == nil && a.Address == s && !strings.ContainsAny(s, "\r\n")
}

func validURL(s string, optional bool) bool {
	if s == "" {
		return optional
	}
	u, err := url.Parse(s)
	return err == nil && utf8.ValidString(s) && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && !strings.ContainsAny(s, " \t\r\n")
}

var hexColor = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)

func validTimes(created, updated time.Time) bool {
	return !created.IsZero() && !updated.Before(created)
}

func checkTime(now, previous time.Time) error {
	if now.IsZero() || now.Before(previous) {
		return invalid("time")
	}
	return nil
}

func copyTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := *t
	return &v
}

func copyID(id *int64) *int64 {
	if id == nil {
		return nil
	}
	v := *id
	return &v
}
