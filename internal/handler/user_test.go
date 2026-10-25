package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUserEndpoints(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		body   string
		status int
		want   User
	}{
		{"get user", http.MethodGet, "/users/1", "", 200, mockUser()},
		{"get profile", http.MethodGet, "/profile", "", 200, mockUser()},
		{"create user", http.MethodPost, "/users", `{"name":"Роман","email":"roman@example.com"}`, 201, User{2, "Роман", "roman@example.com"}},
		{"update user", http.MethodPut, "/users/1", `{"name":"Рома","email":"roma@example.com"}`, 200, User{1, "Рома", "roma@example.com"}},
		{"missing user", http.MethodGet, "/users/999", "", 404, User{}},
		{"update missing user", http.MethodPut, "/users/999", `{"name":"Рома","email":"roma@example.com"}`, 404, User{}},
		{"invalid id", http.MethodGet, "/users/abc", "", 400, User{}},
		{"zero id", http.MethodGet, "/users/0", "", 400, User{}},
		{"negative id", http.MethodPut, "/users/-1", `{}`, 400, User{}},
		{"broken json", http.MethodPost, "/users", `{`, 400, User{}},
		{"empty body", http.MethodPost, "/users", "", 400, User{}},
		{"missing name", http.MethodPost, "/users", `{"email":"roman@example.com"}`, 400, User{}},
		{"missing email", http.MethodPut, "/users/1", `{"name":"Рома"}`, 400, User{}},
		{"invalid email", http.MethodPost, "/users", `{"name":"Рома","email":"invalid"}`, 400, User{}},
	}

	h := New()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			request.Header.Set("Content-Type", "application/json")
			h.router.ServeHTTP(response, request)
			if response.Code != tt.status {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, tt.status, response.Body.String())
			}
			if !strings.HasPrefix(response.Header().Get("Content-Type"), "application/json") {
				t.Fatal("response must be JSON")
			}
			if tt.status >= 400 {
				var body struct {
					Error string `json:"error"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Error == "" {
					t.Fatalf("expected JSON error, got %s", response.Body.String())
				}
				return
			}
			var user User
			if err := json.Unmarshal(response.Body.Bytes(), &user); err != nil {
				t.Fatal(err)
			}
			if user != tt.want {
				t.Fatalf("user = %#v, want %#v", user, tt.want)
			}
		})
	}
}

func TestUserMocksDoNotPersist(t *testing.T) {
	h := New()
	for _, operation := range []struct{ method, path string }{
		{http.MethodPost, "/users"},
		{http.MethodPut, "/users/1"},
	} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(operation.method, operation.path, strings.NewReader(`{"name":"Изменён","email":"changed@example.com"}`))
		h.router.ServeHTTP(response, request)
		if response.Code != http.StatusOK && response.Code != http.StatusCreated {
			t.Fatalf("mock write failed: %s", response.Body.String())
		}
	}
	for _, path := range []string{"/users/1", "/profile"} {
		response := httptest.NewRecorder()
		h.router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		var user User
		if err := json.Unmarshal(response.Body.Bytes(), &user); err != nil {
			t.Fatal(err)
		}
		if response.Code != http.StatusOK || user != mockUser() {
			t.Fatalf("mock changed after write: %s", response.Body.String())
		}
	}
	response := httptest.NewRecorder()
	h.router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/users/2", nil))
	if response.Code != http.StatusNotFound {
		t.Fatal("mock creation must not persist a user")
	}
}
