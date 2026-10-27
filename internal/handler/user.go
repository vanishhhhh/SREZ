package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type User struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type userRequest struct {
	Name  string `json:"name" binding:"required"`
	Email string `json:"email" binding:"required,email"`
}

func mockUser() User {
	return User{ID: 1, Name: "Тестовый пользователь", Email: "demo@example.com"}
}
git --no-pager diff -- internal/handler/user.go
func GetUser(c *gin.Context) {
	id, ok := userID(c)
	if !ok {
		return
	}
	user := mockUser()
	if id != user.ID {
		c.JSON(http.StatusNotFound, gin.H{"error": "пользователь не найден"})
		return
	}
	c.JSON(http.StatusOK, user)
}

// CreateUser возвращает пример создания, но пока ничего не сохраняет.
func CreateUser(c *gin.Context) {
	var request userRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "нужны name и корректный email в JSON"})
		return
	}
	c.JSON(http.StatusCreated, User{ID: 2, Name: request.Name, Email: request.Email})
}

// UpdateUser возвращает переданные данные, но пока не изменяет заглушку.
func UpdateUser(c *gin.Context) {
	id, ok := userID(c)
	if !ok {
		return
	}
	if id != mockUser().ID {
		c.JSON(http.StatusNotFound, gin.H{"error": "пользователь не найден"})
		return
	}
	var request userRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "нужны name и корректный email в JSON"})
		return
	}
	c.JSON(http.StatusOK, User{ID: id, Name: request.Name, Email: request.Email})
}

// GetProfile пока возвращает тестовый профиль без авторизации.
func GetProfile(c *gin.Context) {
	c.JSON(http.StatusOK, mockUser())
}

func userID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id должен быть положительным целым числом"})
		return 0, false
	}
	return id, true
}
