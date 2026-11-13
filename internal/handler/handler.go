package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"srez/internal/usecase"
)

type Handler struct {
	router     *gin.Engine
	users      *usecase.Users
	brands     *usecase.Brands
	products   *usecase.Products
	management *usecase.Management
}

func NewWithUsers(users *usecase.Users) *Handler {
	return New(users, nil, nil)
}

func New(users *usecase.Users, brands *usecase.Brands, products *usecase.Products) *Handler {
	router := gin.New()
	router.Use(gin.LoggerWithFormatter(safeAccessLog), safeRecovery(gin.DefaultErrorWriter))
	_ = router.SetTrustedProxies(nil)
	h := &Handler{router: router, users: users, brands: brands, products: products}
	router.GET("/projects", GetProjects)
	router.GET("/health", GetHealth)
	if brands != nil && products != nil {
		router.GET("/products", h.GetProducts)
		router.GET("/products/:id", h.GetProduct)
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
