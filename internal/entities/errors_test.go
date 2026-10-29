package entities_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"srez/internal/entities"
)

var created = time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)
var changed = created.Add(time.Hour)

func noError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func unchanged(t *testing.T, before, after any, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("operation mutated state:\nbefore: %#v\nafter: %#v", before, after)
	}
}

func same(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestErrorsSupportIs(t *testing.T) {
	_, err := entities.NewUser("bad", "user", "User", "hash", created)
	if !errors.Is(err, entities.ErrInvalidValue) {
		t.Fatalf("error = %v", err)
	}
}
