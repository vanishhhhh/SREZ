package handler

import (
	"bytes"
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

func (h *Handler) ListUsers(c *gin.Context) {
	users, err := h.users.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "не удалось получить список пользователей"})
		return
	}
	response := make([]publicUserResponse, 0, len(users))
	for _, user := range users {
		response = append(response, publicUserDTO(user))
	}
	c.JSON(http.StatusOK, response)
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

func (h *Handler) UpdateUser(c *gin.Context) {
	id, ok := userID(c)
	if !ok {
		return
	}
	var request updateUserRequest
	if !decodeUserRequest(c, &request) {
		return
	}
	user, err := h.users.Update(c.Request.Context(), id, request.DisplayName, request.AvatarURL)
	switch {
	case errors.Is(err, usecase.ErrUserNotFound):
		c.JSON(http.StatusNotFound, errorResponse{Error: "пользователь не найден"})
	case errors.Is(err, usecase.ErrEmptyUserUpdate):
		c.JSON(http.StatusBadRequest, errorResponse{Error: "нужны display_name или avatar_url"})
	case errors.Is(err, entities.ErrInvalidValue):
		c.JSON(http.StatusBadRequest, errorResponse{Error: "некорректное display_name или avatar_url"})
	case err != nil:
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "не удалось обновить пользователя"})
	default:
		c.JSON(http.StatusOK, publicUserDTO(*user))
	}
}

func (h *Handler) DeleteUser(c *gin.Context) {
	id, ok := userID(c)
	if !ok {
		return
	}
	err := h.users.Delete(c.Request.Context(), id)
	switch {
	case errors.Is(err, usecase.ErrUserNotFound):
		c.JSON(http.StatusNotFound, errorResponse{Error: "пользователь не найден"})
	case err != nil:
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "не удалось удалить пользователя"})
	default:
		c.Status(http.StatusNoContent)
	}
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
