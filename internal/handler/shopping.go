package handler

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"srez/internal/entities"
	"srez/internal/usecase"
)

func (h *Handler) SetShopping(s *usecase.Shopping) { h.shopping = s }
func (h *Handler) shoppingRoutes(x *authHTTP) {
	add := func(method, path string, fn gin.HandlerFunc) {
		h.router.Handle(method, path, x.bearer, func(c *gin.Context) {
			if h.shopping == nil {
				c.AbortWithStatusJSON(503, errorResponse{"shopping unavailable"})
				return
			}
			c.Next()
		}, fn)
	}
	add("GET", "/favorites", h.favorites)
	add("POST", "/favorites/:product_id", h.addFavorite)
	add("DELETE", "/favorites/:product_id", h.removeFavorite)
	add("GET", "/cart", h.cart)
	add("POST", "/cart/items", h.addCartItem)
	add("PATCH", "/cart/items/:id", h.setCartItem)
	add("DELETE", "/cart/items/:id", h.removeCartItem)
}
func shoppingHTTPError(c *gin.Context, err error) {
	status, message := 500, "shopping operation failed"
	switch {
	case errors.Is(err, usecase.ErrEmptyCart):
		status, message = 409, "cart is empty"
	case errors.Is(err, usecase.ErrResourceHidden):
		status, message = 404, "resource not found"
	case errors.Is(err, entities.ErrInvalidValue):
		status, message = 400, "invalid shopping data"
	case errors.Is(err, usecase.ErrInsufficientStock):
		status, message = 409, "insufficient stock"
	case errors.Is(err, usecase.ErrCartUnavailable):
		status, message = 409, "cart contains unavailable items"
	case errors.Is(err, entities.ErrMoneyOverflow):
		status, message = 409, "money exceeds supported limit"
	}
	c.JSON(status, errorResponse{message})
}

type favoriteResponse struct {
	ProductID int64                `json:"product_id"`
	Product   *productCardResponse `json:"product"`
}

func (h *Handler) favorites(c *gin.Context) {
	items, err := h.shopping.Favorites(c.Request.Context(), actor(c))
	if err != nil {
		shoppingHTTPError(c, err)
		return
	}
	result := make([]favoriteResponse, 0, len(items))
	for _, item := range items {
		r := favoriteResponse{ProductID: item.ProductID}
		if item.Product != nil {
			card := productCardDTO(*item.Product)
			r.Product = &card
		}
		result = append(result, r)
	}
	c.JSON(200, result)
}
func favoriteID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("product_id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(400, errorResponse{"invalid product ID"})
		return 0, false
	}
	return id, true
}
func (h *Handler) addFavorite(c *gin.Context) {
	id, ok := favoriteID(c)
	if !ok {
		return
	}
	if err := h.shopping.AddFavorite(c.Request.Context(), actor(c), id); err != nil {
		shoppingHTTPError(c, err)
		return
	}
	c.Status(204)
}
func (h *Handler) removeFavorite(c *gin.Context) {
	id, ok := favoriteID(c)
	if !ok {
		return
	}
	if err := h.shopping.RemoveFavorite(c.Request.Context(), actor(c), id); err != nil {
		shoppingHTTPError(c, err)
		return
	}
	c.Status(204)
}

type cartProductResponse struct {
	ProductID        int64  `json:"product_id"`
	BrandID          int64  `json:"brand_id"`
	Name             string `json:"name"`
	PreviewImageURL  string `json:"preview_image_url"`
	Color            string `json:"color"`
	Size             string `json:"size"`
	UnitPriceKopecks int64  `json:"unit_price_kopecks"`
}
type cartLineResponse struct {
	ID               int64                `json:"id"`
	ProductVariantID *int64               `json:"product_variant_id"`
	Quantity         int64                `json:"quantity"`
	Product          *cartProductResponse `json:"product"`
	Availability     string               `json:"availability"`
	SubtotalKopecks  int64                `json:"subtotal_kopecks"`
}
type cartResponse struct {
	Items        []cartLineResponse `json:"items"`
	TotalKopecks int64              `json:"total_kopecks"`
}

func cartDTO(cart *usecase.Cart) cartResponse {
	r := cartResponse{Items: make([]cartLineResponse, 0, len(cart.Items)), TotalKopecks: cart.TotalKopecks}
	for _, line := range cart.Items {
		item := cartLineResponse{ID: line.ID, ProductVariantID: line.VariantID, Quantity: line.Quantity, Availability: line.Availability, SubtotalKopecks: line.SubtotalKopecks}
		if p := line.Product; p != nil {
			item.Product = &cartProductResponse{ProductID: p.ProductID, BrandID: p.BrandID, Name: p.Name, PreviewImageURL: p.PreviewImageURL, Color: p.Color, Size: p.Size, UnitPriceKopecks: p.UnitPriceKopecks}
		}
		r.Items = append(r.Items, item)
	}
	return r
}
func (h *Handler) cart(c *gin.Context) {
	cart, err := h.shopping.Cart(c.Request.Context(), actor(c))
	if err != nil {
		shoppingHTTPError(c, err)
		return
	}
	c.JSON(200, cartDTO(cart))
}
func (h *Handler) addCartItem(c *gin.Context) {
	var r struct {
		VariantID int64 `json:"product_variant_id"`
		Quantity  int64 `json:"quantity"`
	}
	if !decodeUserRequest(c, &r) {
		return
	}
	cart, err := h.shopping.ChangeCart(c.Request.Context(), actor(c), r.VariantID, r.Quantity, true)
	if err != nil {
		shoppingHTTPError(c, err)
		return
	}
	c.JSON(200, cartDTO(cart))
}
func (h *Handler) setCartItem(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	var r struct {
		Quantity int64 `json:"quantity"`
	}
	if !decodeUserRequest(c, &r) {
		return
	}
	cart, err := h.shopping.ChangeCart(c.Request.Context(), actor(c), id, r.Quantity, false)
	if err != nil {
		shoppingHTTPError(c, err)
		return
	}
	c.JSON(200, cartDTO(cart))
}
func (h *Handler) removeCartItem(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	if err := h.shopping.RemoveCartItem(c.Request.Context(), actor(c), id); err != nil {
		shoppingHTTPError(c, err)
		return
	}
	c.Status(204)
}
