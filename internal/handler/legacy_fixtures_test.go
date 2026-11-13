package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"srez/internal/entities"
)

// Placeholder for domain validation in examples, never for authentication.
const mockUserPasswordHash = "mock-password-hash-not-for-authentication"

func mockUser() (*entities.User, error) {
	now := time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)
	return entities.RestoreUser(entities.UserState{
		ID: 1, Email: "demo@example.com", Username: "demo_user", DisplayName: "Тестовый пользователь",
		PasswordHash: mockUserPasswordHash, AvatarURL: "https://example.com/users/1/avatar.png", CreatedAt: now, UpdatedAt: now,
	})
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
