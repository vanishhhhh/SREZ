package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	router *gin.Engine
}

func New() *Handler {
	router := gin.Default()
	router.GET("/projects", GetProjects)
	router.GET("/users/:id", GetUser)
	router.POST("/users", CreateUser)
	router.PUT("/users/:id", UpdateUser)
	router.GET("/profile", GetProfile)
	router.GET("/health", GetHealth)
	router.GET("/products", GetProducts)
	router.GET("/products/:id", GetProduct)
	router.POST("/products", CreateProduct)
	router.GET("/brands", GetBrands)
	router.GET("/brands/:slug", GetBrand)

	return &Handler{router: router}
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
