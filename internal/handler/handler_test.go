package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestGetProjects(t *testing.T) {
	h := New()
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/projects", nil)

	h.router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}

	type project struct {
		ID    int    `json:"id"`
		Title string `json:"title"`
	}
	var projects []project
	if err := json.Unmarshal(response.Body.Bytes(), &projects); err != nil {
		t.Fatalf("decode projects: %v", err)
	}
	want := []project{{ID: 1, Title: "Дело"}, {ID: 2, Title: "Витрина"}}
	if !reflect.DeepEqual(projects, want) {
		t.Fatalf("projects = %#v, want %#v", projects, want)
	}
}
