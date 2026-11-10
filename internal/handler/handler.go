package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"srez/internal/usecase"
)

type Handler struct {
	router *gin.Engine
	users  *usecase.Users
}

func NewWithUsers(users *usecase.Users) *Handler {
	router := gin.Default()
	h := &Handler{router: router, users: users}
	router.GET("/projects", GetProjects)
	router.GET("/users", h.ListUsers)
	router.GET("/users/:id", h.GetUser)
	router.POST("/users", h.CreateUser)
	router.PUT("/users/:id", h.UpdateUser)
	router.DELETE("/users/:id", h.DeleteUser)
	router.GET("/profile", GetProfile)
	router.GET("/health", GetHealth)
	router.GET("/products", GetProducts)
	router.GET("/products/:id", GetProduct)
	router.POST("/products", CreateProduct)
	router.GET("/brands", GetBrands)
	router.GET("/brands/:slug", GetBrand)

	return h
}

func (h *Handler) Run(addr string) error {
	return h.router.Run(addr)
}

func GetProjects(c *gin.Context) {
	c.JSON(http.StatusOK, []gin.H{
		{"id": 1, "title": "Дело"},
		{"id": 2, "title": "Витрина"},
	})
}
