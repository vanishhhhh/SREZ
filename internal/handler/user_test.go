package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"srez/internal/entities"
	"srez/internal/usecase"
)

func newTestHandler() *Handler {
	return NewWithUsers(usecase.NewUsers(mockUserRepository{}))
}

type mockUserRepository struct{}

func (mockUserRepository) List(_ context.Context) ([]entities.User, error) {
	user, err := mockUser()
	if err != nil {
		return nil, err
	}
	return []entities.User{*user}, nil
}

func (mockUserRepository) Get(_ context.Context, id int64) (*entities.User, error) {
	if id != 1 {
		return nil, nil
	}
	return mockUser()
}

func (mockUserRepository) Insert(_ context.Context, user entities.User) (*entities.User, error) {
	state := user.State()
	state.ID = 2
	return entities.RestoreUser(state)
}

func (r mockUserRepository) Update(ctx context.Context, id int64, displayName, avatarURL *string, now time.Time) (*entities.User, error) {
	user, err := r.Get(ctx, id)
	if err != nil || user == nil {
		return user, err
	}
	state := user.State()
	if displayName != nil {
		state.DisplayName = *displayName
	}
	if avatarURL != nil {
		state.AvatarURL = *avatarURL
	}
	state.UpdatedAt = now
	return entities.RestoreUser(state)
}

func (mockUserRepository) Delete(_ context.Context, id int64) (bool, error) {
	return id == 1, nil
}

const publicMockUserJSON = `{"id":1,"username":"demo_user","display_name":"Тестовый пользователь","avatar_url":"https://example.com/users/1/avatar.png"}`
const profileMockUserJSON = `{"id":1,"email":"demo@example.com","username":"demo_user","display_name":"Тестовый пользователь","avatar_url":"https://example.com/users/1/avatar.png"}`
const createMockUserJSON = `{"email":"roman@example.com","username":"roman_1","display_name":" Роман "}`

func TestUserEndpoints(t *testing.T) {
	h := newTestHandler()
	for _, tt := range []struct {
		name, method, path, body string
		status                   int
		want                     string
	}{
		{"public user", "GET", "/users/1", "", 200, publicMockUserJSON},
		{"own profile", "GET", "/profile", "", 200, profileMockUserJSON},
		{"create", "POST", "/users", createMockUserJSON, 201, `{"id":2,"username":"roman_1","display_name":"Роман","avatar_url":""}`},
		{"change both", "PUT", "/users/1", `{"display_name":" Новое имя ","avatar_url":"https://example.com/new.png"}`, 200, `{"id":1,"username":"demo_user","display_name":"Новое имя","avatar_url":"https://example.com/new.png"}`},
		{"name only", "PUT", "/users/1", `{"display_name":"Рома"}`, 200, `{"id":1,"username":"demo_user","display_name":"Рома","avatar_url":"https://example.com/users/1/avatar.png"}`},
		{"clear avatar", "PUT", "/users/1", `{"avatar_url":""}`, 200, `{"id":1,"username":"demo_user","display_name":"Тестовый пользователь","avatar_url":""}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			response := catalogRequest(t, h, tt.method, tt.path, tt.body)
			if response.Code != tt.status {
				t.Fatalf("status = %d: %s", response.Code, response.Body.String())
			}
			assertCatalogJSON(t, response.Body.Bytes(), tt.want)
		})
	}
}

func TestUserIDs(t *testing.T) {
	h := newTestHandler()
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		for _, tt := range []struct {
			id     string
			status int
		}{
			{"abc", 400}, {"0", 400}, {"-1", 400}, {"1.5", 400}, {"9223372036854775808", 400}, {"9223372036854775807", 404}, {"2", 404}, {"999", 404},
		} {
			t.Run(method+tt.id, func(t *testing.T) {
				assertCatalogError(t, catalogRequest(t, h, method, "/users/"+tt.id, `{"display_name":"Рома"}`), tt.status)
			})
		}
	}
}

func TestCreateUserErrors(t *testing.T) {
	h := newTestHandler()
	for _, tt := range []struct{ name, body string }{
		{"empty", ""}, {"broken", "{"}, {"null", "null"}, {"array", "[]"}, {"scalar", "true"}, {"missing", "{}"},
		{"two objects", createMockUserJSON + " {}"}, {"trailing", createMockUserJSON + " garbage"},
		{"bad email", `{"email":"bad","username":"roman","display_name":"Имя"}`},
		{"missing email", `{"username":"roman","display_name":"Имя"}`},
		{"email name", `{"email":"Name <r@example.com>","username":"roman","display_name":"Имя"}`},
		{"short username", `{"email":"r@example.com","username":"ab","display_name":"Имя"}`},
		{"long username", `{"email":"r@example.com","username":"` + strings.Repeat("a", 31) + `","display_name":"Имя"}`},
		{"unicode username", `{"email":"r@example.com","username":"роман","display_name":"Имя"}`},
		{"missing username", `{"email":"r@example.com","display_name":"Имя"}`},
		{"blank display", `{"email":"r@example.com","username":"roman","display_name":" \t"}`},
		{"missing display", `{"email":"r@example.com","username":"roman"}`},
		{"wrong type", `{"email":"r@example.com","username":1,"display_name":"Имя"}`},
	} {
		t.Run(tt.name, func(t *testing.T) { assertCatalogError(t, catalogRequest(t, h, "POST", "/users", tt.body), 400) })
	}
	for _, field := range []string{"name", "password", "password_hash", "id", "avatar_url", "extra"} {
		t.Run("unknown/"+field, func(t *testing.T) {
			body := strings.TrimSuffix(createMockUserJSON, "}") + `,"` + field + `":"secret"}`
			assertCatalogError(t, catalogRequest(t, h, "POST", "/users", body), 400)
		})
	}
}

func TestUpdateUserErrors(t *testing.T) {
	h := newTestHandler()
	for _, tt := range []struct{ name, body string }{
		{"empty", ""}, {"broken", "{"}, {"null", "null"}, {"array", "[]"}, {"missing", "{}"}, {"null field", `{"display_name":null}`},
		{"blank display", `{"display_name":" \t"}`}, {"wrong type", `{"display_name":1}`},
		{"http avatar", `{"avatar_url":"http://example.com/a"}`}, {"relative avatar", `{"avatar_url":"/a"}`},
		{"missing host", `{"avatar_url":"https:///a"}`},
		{"partial failure", `{"display_name":"Новое имя","avatar_url":"bad"}`},
		{"two objects", `{"display_name":"Имя"} {}`}, {"trailing", `{"display_name":"Имя"} garbage`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assertCatalogError(t, catalogRequest(t, h, "PUT", "/users/1", tt.body), 400)
			assertCatalogJSON(t, catalogRequest(t, h, "GET", "/users/1", "").Body.Bytes(), publicMockUserJSON)
			assertCatalogJSON(t, catalogRequest(t, h, "GET", "/profile", "").Body.Bytes(), profileMockUserJSON)
		})
	}
	for _, field := range []string{"email", "username", "password", "password_hash", "id", "name", "extra"} {
		t.Run("forbidden/"+field, func(t *testing.T) {
			body := `{"display_name":"Имя","` + field + `":"secret"}`
			assertCatalogError(t, catalogRequest(t, h, "PUT", "/users/1", body), 400)
		})
	}
}

func TestUserMocksDoNotPersist(t *testing.T) {
	h := newTestHandler()
	for range 2 {
		created := catalogRequest(t, h, "POST", "/users", createMockUserJSON)
		if created.Code != 201 {
			t.Fatalf("create status = %d", created.Code)
		}
		assertCatalogError(t, catalogRequest(t, h, "GET", "/users/2", ""), 404)
		updated := catalogRequest(t, h, "PUT", "/users/1", `{"display_name":"Изменён","avatar_url":""}`)
		if updated.Code != 200 {
			t.Fatalf("update status = %d", updated.Code)
		}
		assertCatalogJSON(t, catalogRequest(t, h, "GET", "/users/1", "").Body.Bytes(), publicMockUserJSON)
		assertCatalogJSON(t, catalogRequest(t, h, "GET", "/profile", "").Body.Bytes(), profileMockUserJSON)
		assertCatalogJSON(t, catalogRequest(t, h, "PUT", "/users/1", `{"avatar_url":""}`).Body.Bytes(), `{"id":1,"username":"demo_user","display_name":"Тестовый пользователь","avatar_url":""}`)
	}
}

func TestUserBodyLimits(t *testing.T) {
	const limit = 65536
	for _, endpoint := range []struct {
		method, path, valid, want string
		success                   int
	}{
		{http.MethodPost, "/users", createMockUserJSON, `{"id":2,"username":"roman_1","display_name":"Роман","avatar_url":""}`, http.StatusCreated},
		{http.MethodPut, "/users/1", `{"avatar_url":""}`, `{"id":1,"username":"demo_user","display_name":"Тестовый пользователь","avatar_url":""}`, http.StatusOK},
	} {
		for _, tt := range []struct {
			name, body string
			status     int
		}{
			{"65536 bytes", endpoint.valid + strings.Repeat(" ", limit-len(endpoint.valid)), endpoint.success},
			{"65537 bytes", endpoint.valid + strings.Repeat(" ", limit-len(endpoint.valid)+1), http.StatusRequestEntityTooLarge},
			{"malformed 65536 bytes", "!" + strings.Repeat(" ", limit-1), http.StatusBadRequest},
			{"malformed 65537 bytes", "!" + strings.Repeat(" ", limit), http.StatusRequestEntityTooLarge},
			{"second object", endpoint.valid + " {}", http.StatusBadRequest},
		} {
			for _, unknownLength := range []bool{false, true} {
				t.Run(endpoint.method+"/"+tt.name+map[bool]string{false: "/known length", true: "/unknown length"}[unknownLength], func(t *testing.T) {
					h := newTestHandler()
					request := httptest.NewRequest(endpoint.method, endpoint.path, strings.NewReader(tt.body))
					request.Header.Set("Content-Type", "application/json")
					if unknownLength {
						request.ContentLength = -1
						request.TransferEncoding = []string{"chunked"}
					}
					response := httptest.NewRecorder()
					h.router.ServeHTTP(response, request)
					if !strings.HasPrefix(response.Header().Get("Content-Type"), "application/json") {
						t.Fatal("expected JSON response")
					}
					if tt.status >= 400 {
						assertCatalogError(t, response, tt.status)
					} else {
						if response.Code != tt.status {
							t.Fatalf("status = %d, want %d: %s", response.Code, tt.status, response.Body.String())
						}
						assertCatalogJSON(t, response.Body.Bytes(), endpoint.want)
					}
					assertCatalogJSON(t, catalogRequest(t, h, http.MethodGet, "/users/1", "").Body.Bytes(), publicMockUserJSON)
					assertCatalogJSON(t, catalogRequest(t, h, http.MethodGet, "/profile", "").Body.Bytes(), profileMockUserJSON)
				})
			}
		}
	}
}
