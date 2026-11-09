package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"srez/internal/entities"
	"srez/internal/usecase"
)

type createUserRequest struct {
	Email       string `json:"email"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}

type updateUserRequest struct {
	DisplayName *string `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
}

type publicUserResponse struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
}

type profileResponse struct {
	publicUserResponse
	Email string `json:"email"`
}

func publicUserDTO(user entities.User) publicUserResponse {
	return publicUserResponse{ID: user.ID(), Username: user.Username(), DisplayName: user.DisplayName(), AvatarURL: user.AvatarURL()}
}

// Placeholder for domain validation in examples, never for authentication.
const mockUserPasswordHash = "mock-password-hash-not-for-authentication"

func mockUser() (*entities.User, error) {
	now := time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)
	return entities.RestoreUser(entities.UserState{
		ID: 1, Email: "demo@example.com", Username: "demo_user", DisplayName: "Тестовый пользователь",
		PasswordHash: mockUserPasswordHash, AvatarURL: "https://example.com/users/1/avatar.png", CreatedAt: now, UpdatedAt: now,
	})
}

type mockUserRepository struct{}

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

// Strict decoding is local to User endpoints; Gin settings are unchanged.
func decodeUserRequest(c *gin.Context, target any) bool {
	body, ok := readJSONBody(c)
	if !ok {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "нужен JSON-объект с допустимыми полями пользователя"})
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "нужен ровно один JSON-объект"})
		return false
	}
	return true
}

func (h *Handler) GetUser(c *gin.Context) {
	id, ok := userID(c)
	if !ok {
		return
	}
	user, err := h.users.Get(c.Request.Context(), id)
	if errors.Is(err, usecase.ErrUserNotFound) {
		c.JSON(http.StatusNotFound, errorResponse{Error: "пользователь не найден"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "не удалось получить пользователя"})
		return
	}
	c.JSON(http.StatusOK, publicUserDTO(*user))
}

func (h *Handler) CreateUser(c *gin.Context) {
	var request createUserRequest
	if !decodeUserRequest(c, &request) {
		return
	}
	user, err := h.users.Create(c.Request.Context(), request.Email, request.Username, request.DisplayName)
	switch {
	case errors.Is(err, entities.ErrInvalidValue):
		c.JSON(http.StatusBadRequest, errorResponse{Error: "некорректные данные пользователя"})
	case errors.Is(err, usecase.ErrUsernameTaken):
		c.JSON(http.StatusConflict, errorResponse{Error: "имя пользователя уже занято"})
	case err != nil:
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "не удалось создать пользователя"})
	default:
		c.JSON(http.StatusCreated, publicUserDTO(*user))
	}
}

// UpdateUser changes only a fresh copy of the fixed example.
func UpdateUser(c *gin.Context) {
	id, ok := userID(c)
	if !ok {
		return
	}
	user, err := mockUser()
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "не удалось подготовить тестового пользователя"})
		return
	}
	if id != user.ID() {
		c.JSON(http.StatusNotFound, errorResponse{Error: "пользователь не найден"})
		return
	}
	var request updateUserRequest
	if !decodeUserRequest(c, &request) {
		return
	}
	if request.DisplayName == nil && request.AvatarURL == nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "нужны display_name или avatar_url"})
		return
	}
	now := time.Now().UTC()
	if request.DisplayName != nil {
		if err := user.ChangeDisplayName(*request.DisplayName, now); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{Error: "некорректное display_name"})
			return
		}
	}
	if request.AvatarURL != nil {
		if err := user.ChangeAvatar(*request.AvatarURL, now); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{Error: "некорректный avatar_url"})
			return
		}
	}
	c.JSON(http.StatusOK, publicUserDTO(*user))
}

// GetProfile is a fixed own-profile example without authorization or sessions.
func GetProfile(c *gin.Context) {
	user, err := mockUser()
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "не удалось подготовить тестового пользователя"})
		return
	}
	c.JSON(http.StatusOK, profileResponse{publicUserResponse: publicUserDTO(*user), Email: user.Email()})
}

func userID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "id должен быть положительным целым числом int64"})
		return 0, false
	}
	return id, true
}
