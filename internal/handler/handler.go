package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"srez/internal/usecase"
)

type Handler struct {
	router     *gin.Engine
	users      *usecase.Users
	brands     *usecase.Brands
	products   *usecase.Products
	management *usecase.Management
	shopping   *usecase.Shopping
	orders     *usecase.Orders
	media      *usecase.Media
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
	server := &http.Server{Addr: addr, Handler: h.router, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024}
	return server.ListenAndServe()
}

func GetProjects(c *gin.Context) {
	c.JSON(http.StatusOK, []gin.H{
		{"id": 1, "title": "Дело"},
		{"id": 2, "title": "Витрина"},
	})
}
