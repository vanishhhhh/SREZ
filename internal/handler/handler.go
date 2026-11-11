package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"srez/internal/usecase"
)

type Handler struct {
	router          *gin.Engine
	users           *usecase.Users
	brands          *usecase.Brands
	products        *usecase.Products
	devCatalogWrite bool
}

func NewWithUsers(users *usecase.Users) *Handler {
	return New(users, nil, nil, false)
}

func New(users *usecase.Users, brands *usecase.Brands, products *usecase.Products, devCatalogWrite bool) *Handler {
	router := gin.Default()
	h := &Handler{router: router, users: users, brands: brands, products: products, devCatalogWrite: devCatalogWrite}
	router.GET("/projects", GetProjects)
	router.GET("/users", h.ListUsers)
	router.GET("/users/:id", h.GetUser)
	router.POST("/users", h.CreateUser)
	router.PUT("/users/:id", h.UpdateUser)
	router.DELETE("/users/:id", h.DeleteUser)
	router.GET("/profile", GetProfile)
	router.GET("/health", GetHealth)
	if brands != nil && products != nil {
		router.GET("/products", h.GetProducts)
		router.GET("/products/:id", h.GetProduct)
		router.POST("/products", h.CreateProduct)
		router.GET("/brands", h.GetBrands)
		router.GET("/brands/:slug", h.GetBrand)
	}

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
