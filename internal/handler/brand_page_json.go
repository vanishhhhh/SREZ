package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
)

var errBrandPageJSON = errors.New("invalid brand page JSON structure")

// encoding/json supports case-insensitive field matching and duplicate keys.
// This endpoint's versioned DTOs require a single canonical representation.
func checkBrandPageJSON(body []byte, target any) error {
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	if err := checkBrandPageValue(d, reflect.TypeOf(target), 0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errBrandPageJSON
	}
	return nil
}

func checkBrandPageValue(d *json.Decoder, shape reflect.Type, depth int) error {
	if depth > 16 {
		return errBrandPageJSON
	}
	for shape.Kind() == reflect.Pointer {
		shape = shape.Elem()
	}
	token, err := d.Token()
	if err != nil {
		return err
	}
	switch token {
	case json.Delim('{'):
		if shape.Kind() != reflect.Struct {
			return errBrandPageJSON
		}
		fields := make(map[string]reflect.Type, shape.NumField())
		for i := 0; i < shape.NumField(); i++ {
			field := shape.Field(i)
			key, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			if key != "" && key != "-" {
				fields[key] = field.Type
			}
		}
		for d.More() {
			token, err = d.Token()
			key, ok := token.(string)
			child := fields[key]
			if err != nil || !ok || child == nil {
				return errBrandPageJSON
			}
			delete(fields, key)
			if err = checkBrandPageValue(d, child, depth+1); err != nil {
				return err
			}
		}
		if end, e := d.Token(); e != nil || end != json.Delim('}') {
			return errBrandPageJSON
		}
	case json.Delim('['):
		if shape.Kind() != reflect.Slice && shape.Kind() != reflect.Array {
			return errBrandPageJSON
		}
		for d.More() {
			if err = checkBrandPageValue(d, shape.Elem(), depth+1); err != nil {
				return err
			}
		}
		if end, e := d.Token(); e != nil || end != json.Delim(']') {
			return errBrandPageJSON
		}
	}
	return nil
}
