package entities_test

import (
	"fmt"
	"srez/internal/entities"
	"testing"
)

func imageURLs(n int) []string {
	s := make([]string, n)
	for i := range s {
		s[i] = fmt.Sprintf("https://example.com/%d", i)
	}
	return s
}
func color(t *testing.T) *entities.ProductColor {
	t.Helper()
	c, err := entities.NewProductColor(10, "Красный", imageURLs(1))
	noError(t, err)
	return c
}

func TestNewProductColor(t *testing.T) {
	for _, tt := range []struct {
		name    string
		product int64
		color   string
		images  []string
		valid   bool
	}{
		{"empty gallery", 10, "Красный", nil, true}, {"max gallery", 10, "Цвет", imageURLs(10), true},
		{"bad product", 0, "Цвет", nil, false}, {"empty color", 10, " ", nil, false},
		{"too many", 10, "Цвет", imageURLs(11), false}, {"empty URL", 10, "Цвет", []string{""}, false},
		{"http URL", 10, "Цвет", []string{"http://example.com/a"}, false}, {"relative URL", 10, "Цвет", []string{"/a"}, false},
		{"duplicates", 10, "Цвет", []string{"https://example.com/a", "https://example.com/a"}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, err := entities.NewProductColor(tt.product, tt.color, tt.images)
			if !tt.valid {
				if err == nil || c != nil {
					t.Fatal("invalid color accepted")
				}
				return
			}
			noError(t, err)
			same(t, c.ID(), int64(0))
			same(t, c.ProductID(), tt.product)
			same(t, c.Color(), tt.color)
			same(t, c.Images(), tt.images)
		})
	}
}

func TestProductColorChanges(t *testing.T) {
	for _, tt := range []struct {
		name       string
		op         func(*entities.ProductColor) error
		valid      bool
		wantColor  string
		wantImages []string
	}{
		{"color", func(c *entities.ProductColor) error { return c.ChangeColor("Синий") }, true, "Синий", imageURLs(1)},
		{"add", func(c *entities.ProductColor) error { return c.AddImage("https://example.com/1") }, true, "Красный", imageURLs(2)},
		{"remove", func(c *entities.ProductColor) error { return c.RemoveImage("https://example.com/0") }, true, "Красный", []string{}},
		{"empty color", func(c *entities.ProductColor) error { return c.ChangeColor(" ") }, false, "", nil},
		{"empty image", func(c *entities.ProductColor) error { return c.AddImage("") }, false, "", nil},
		{"duplicate", func(c *entities.ProductColor) error { return c.AddImage("https://example.com/0") }, false, "", nil},
		{"missing image", func(c *entities.ProductColor) error { return c.RemoveImage("https://example.com/missing") }, false, "", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := color(t)
			before := c.State()
			err := tt.op(c)
			if !tt.valid {
				unchanged(t, before, c.State(), err, entities.ErrInvalidValue)
				return
			}
			noError(t, err)
			same(t, c.Color(), tt.wantColor)
			same(t, c.Images(), tt.wantImages)
		})
	}
	c, err := entities.NewProductColor(10, "Цвет", imageURLs(10))
	noError(t, err)
	before := c.State()
	err = c.AddImage("https://example.com/10")
	unchanged(t, before, c.State(), err, entities.ErrInvalidValue)
}

func TestProductColorSliceIsolationAndRestore(t *testing.T) {
	input := imageURLs(1)
	c, err := entities.NewProductColor(10, "Цвет", input)
	noError(t, err)
	input[0] = "bad"
	same(t, c.Images(), imageURLs(1))
	got := c.Images()
	got[0] = "bad"
	s := c.State()
	s.Images[0] = "bad"
	same(t, c.Images(), imageURLs(1))
	s = c.State()
	s.ID = 20
	restored, err := entities.RestoreProductColor(s)
	noError(t, err)
	s.Images[0] = "bad"
	same(t, restored.Images(), imageURLs(1))
	same(t, restored.ID(), int64(20))
	copy := *restored
	noError(t, copy.AddImage("https://example.com/1"))
	same(t, restored.Images(), imageURLs(1))
	for _, mutate := range []func(*entities.ProductColorState){func(s *entities.ProductColorState) { s.ID = 0 }, func(s *entities.ProductColorState) { s.ProductID = 0 }, func(s *entities.ProductColorState) { s.Images = []string{"bad"} }} {
		s := restored.State()
		mutate(&s)
		if _, err := entities.RestoreProductColor(s); err == nil {
			t.Fatal("invalid restore accepted")
		}
	}
}

func TestProductColorNameNormalization(t *testing.T) {
	for _, input := range []string{" Красный ", "\tКрасный\u00a0"} {
		t.Run(input, func(t *testing.T) {
			c, err := entities.NewProductColor(10, input, imageURLs(1))
			noError(t, err)
			same(t, c.Color(), "Красный")
			s := c.State()
			s.ID = 20
			s.Color = input
			c, err = entities.RestoreProductColor(s)
			noError(t, err)
			same(t, c.Color(), "Красный")
			noError(t, c.ChangeColor(" Синий "))
			same(t, c.Color(), "Синий")
			before := c.State()
			err = c.ChangeColor("\t \u00a0")
			unchanged(t, before, c.State(), err, entities.ErrInvalidValue)
		})
	}
}
