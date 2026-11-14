package entities_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"srez/internal/entities"
)

func TestProductImageValidationAndAtomicChanges(t *testing.T) {
	for _, tt := range []struct {
		name, url string
		colorID   int64
		position  int
		valid     bool
	}{
		{"valid", "https://example.com/a.jpg", 1, 0, true},
		{"last slot", "https://example.com/a.jpg", 1, 9, true},
		{"http", "http://example.com/a.jpg", 1, 0, false},
		{"credentials", "https://user:password@example.com/a.jpg", 1, 0, false},
		{"empty", "", 1, 0, false}, {"bad color", "https://example.com/a.jpg", 0, 0, false},
		{"negative", "https://example.com/a.jpg", 1, -1, false},
		{"overflow slot", "https://example.com/a.jpg", 1, 10, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := entities.NewProductImage(tt.colorID, tt.url, tt.position)
			if (err == nil) != tt.valid {
				t.Fatal(err)
			}
		})
	}
	image, err := entities.RestoreProductImage(entities.ProductImageState{ID: 7, ProductColorID: 2, URL: "https://example.com/a.jpg", Position: 0})
	if err != nil {
		t.Fatal(err)
	}
	before := image.State()
	if err = image.ChangeURL("http://example.com/bad.jpg"); !errors.Is(err, entities.ErrInvalidValue) || !reflect.DeepEqual(before, image.State()) {
		t.Fatal("invalid URL changed state", err)
	}
	if err = image.ChangePosition(10); !errors.Is(err, entities.ErrInvalidValue) || !reflect.DeepEqual(before, image.State()) {
		t.Fatal("invalid position changed state", err)
	}
	if err = image.ChangeURL("https://example.com/new.jpg"); err != nil {
		t.Fatal(err)
	}
	if err = image.ChangePosition(1); err != nil || image.ID() != 7 || image.ProductColorID() != 2 || image.Position() != 1 {
		t.Fatal("identity changed", err)
	}
	before.ID = 0
	if _, err = entities.RestoreProductImage(before); !errors.Is(err, entities.ErrInvalidValue) {
		t.Fatal(err)
	}
}

func TestProductTouchPreservesFieldsAndRejectsEarlierTime(t *testing.T) {
	p := newProduct(t)
	before := p.State()
	if err := p.Touch(before.UpdatedAt.Add(-time.Second)); !errors.Is(err, entities.ErrInvalidValue) || !reflect.DeepEqual(before, p.State()) {
		t.Fatal("invalid touch changed product", err)
	}
	next := before.UpdatedAt.Add(time.Second)
	if err := p.Touch(next); err != nil {
		t.Fatal(err)
	}
	before.UpdatedAt = next
	if !reflect.DeepEqual(before, p.State()) {
		t.Fatal("touch changed product fields")
	}
}
