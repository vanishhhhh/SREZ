package handler

import (
	"time"

	"github.com/gin-gonic/gin"
	"srez/internal/entities"
	"srez/internal/usecase"
)

func (h *Handler) SetOrders(orders *usecase.Orders) { h.orders = orders }
func (h *Handler) orderRoutes(x *authHTTP) {
	add := func(method, path string, fn gin.HandlerFunc) {
		h.router.Handle(method, path, x.bearer, func(c *gin.Context) {
			if h.orders == nil {
				c.AbortWithStatusJSON(503, errorResponse{"orders unavailable"})
				return
			}
			c.Next()
		}, fn)
	}
	add("POST", "/orders", h.checkout)
	add("GET", "/orders", h.ordersList)
	add("GET", "/orders/:id", h.order)
}

type orderLineResponse struct {
	ProductVariantID int64  `json:"product_variant_id"`
	ProductID        int64  `json:"product_id"`
	BrandID          int64  `json:"brand_id"`
	ProductName      string `json:"product_name"`
	Color            string `json:"color"`
	Size             string `json:"size"`
	Quantity         int64  `json:"quantity"`
	UnitPriceKopecks int64  `json:"unit_price_kopecks"`
	SubtotalKopecks  int64  `json:"subtotal_kopecks"`
}
type orderResponse struct {
	ID           int64               `json:"id"`
	Status       string              `json:"status"`
	TotalKopecks int64               `json:"total_kopecks"`
	CreatedAt    time.Time           `json:"created_at"`
	Items        []orderLineResponse `json:"items"`
}

func orderDTO(order entities.Order) orderResponse {
	result := orderResponse{ID: order.ID(), Status: order.Status(), TotalKopecks: order.TotalKopecks(), CreatedAt: order.CreatedAt(), Items: make([]orderLineResponse, 0, len(order.Lines()))}
	for _, line := range order.Lines() {
		// The immutable entity validates the product and total on create/restore.
		amount, _ := entities.LineTotal(line.UnitPriceKopecks, line.Quantity)
		result.Items = append(result.Items, orderLineResponse{ProductVariantID: line.ProductVariantID, ProductID: line.ProductID, BrandID: line.BrandID, ProductName: line.ProductName, Color: line.Color, Size: line.Size, Quantity: line.Quantity, UnitPriceKopecks: line.UnitPriceKopecks, SubtotalKopecks: amount})
	}
	return result
}
func (h *Handler) checkout(c *gin.Context) {
	if !emptyAction(c) {
		return
	}
	order, err := h.orders.Checkout(c.Request.Context(), actor(c))
	if err != nil {
		shoppingHTTPError(c, err)
		return
	}
	c.JSON(201, orderDTO(*order))
}
func (h *Handler) ordersList(c *gin.Context) {
	orders, err := h.orders.List(c.Request.Context(), actor(c))
	if err != nil {
		shoppingHTTPError(c, err)
		return
	}
	result := make([]orderResponse, 0, len(orders))
	for _, order := range orders {
		result = append(result, orderDTO(order))
	}
	c.JSON(200, result)
}
func (h *Handler) order(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	order, err := h.orders.Get(c.Request.Context(), actor(c), id)
	if err != nil {
		shoppingHTTPError(c, err)
		return
	}
	c.JSON(200, orderDTO(*order))
}
