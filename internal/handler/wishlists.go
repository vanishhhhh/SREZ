package handler

import (
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"srez/internal/entities"
	"srez/internal/usecase"
)

func (h *Handler) SetWishlists(s *usecase.Wishlists) { h.wishlists = s }
func (h *Handler) wishlistRoutes(auth *authHTTP) {
	ready := func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("Referrer-Policy", "no-referrer")
		if h.wishlists == nil {
			c.AbortWithStatusJSON(503, errorResponse{"wishlists unavailable"})
			return
		}
		c.Next()
	}
	h.router.GET("/wishlists/shared/:token", ready, h.sharedWishlist)
	add := func(method, path string, fn gin.HandlerFunc) { h.router.Handle(method, path, auth.bearer, ready, fn) }
	add("GET", "/wishlists", h.listWishlists)
	add("POST", "/wishlists", h.createWishlist)
	add("GET", "/wishlists/:id", h.getWishlist)
	add("PATCH", "/wishlists/:id", h.patchWishlist)
	add("DELETE", "/wishlists/:id", h.deleteWishlist)
	add("POST", "/wishlists/:id/items", h.addWishlistItem)
	add("PATCH", "/wishlists/:id/items/:item_id", h.patchWishlistItem)
	add("DELETE", "/wishlists/:id/items/:item_id", h.deleteWishlistItem)
	add("POST", "/wishlists/:id/items/reorder", h.reorderWishlist)
	add("POST", "/wishlists/:id/share", h.shareWishlist)
	add("DELETE", "/wishlists/:id/share", h.unshareWishlist)
}
func wishlistHTTPError(c *gin.Context, err error) {
	status, message := 500, "wishlist operation failed"
	switch {
	case errors.Is(err, usecase.ErrResourceHidden):
		status, message = 404, "resource not found"
	case errors.Is(err, entities.ErrInvalidValue):
		status, message = 400, "invalid wishlist data"
	case errors.Is(err, usecase.ErrWishlistLimit):
		status, message = 409, "wishlist limit reached"
	case errors.Is(err, usecase.ErrWishlistDuplicate):
		status, message = 409, "product already in wishlist"
	case errors.Is(err, usecase.ErrWishlistChanged):
		status, message = 409, "wishlist changed; reload before reordering"
	}
	c.JSON(status, errorResponse{message})
}
func wishlistID(c *gin.Context, key string) (int64, bool) {
	raw := c.Param(key)
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != raw {
		c.JSON(400, errorResponse{"invalid wishlist ID"})
		return 0, false
	}
	return id, true
}

type wishlistSummaryResponse struct {
	ID          int64     `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Visibility  string    `json:"visibility"`
	ItemCount   int       `json:"item_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func wishlistSummaryDTO(w entities.Wishlist) wishlistSummaryResponse {
	return wishlistSummaryResponse{w.ID, w.Title, w.Description, w.Visibility, w.ItemCount, w.CreatedAt, w.UpdatedAt}
}

type wishlistVariantResponse struct {
	ID    int64  `json:"id"`
	Color string `json:"color"`
	Size  string `json:"size"`
	Stock int64  `json:"stock"`
}
type wishlistItemResponse struct {
	ID        int64                    `json:"id"`
	ProductID int64                    `json:"product_id"`
	VariantID *int64                   `json:"product_variant_id"`
	Position  int                      `json:"position"`
	Product   *productCardResponse     `json:"product"`
	Variant   *wishlistVariantResponse `json:"variant"`
}
type wishlistResponse struct {
	wishlistSummaryResponse
	SharePath *string                `json:"share_path"`
	Items     []wishlistItemResponse `json:"items"`
}

func wishlistItemsDTO(items []usecase.WishlistItem) []wishlistItemResponse {
	result := make([]wishlistItemResponse, 0, len(items))
	for _, i := range items {
		r := wishlistItemResponse{ID: i.ID, ProductID: i.ProductID, VariantID: i.VariantID, Position: i.Position}
		if i.Product != nil {
			card := productCardDTO(*i.Product)
			r.Product = &card
		}
		if i.Variant != nil {
			v := i.Variant
			r.Variant = &wishlistVariantResponse{v.ID, v.Color, v.Size, v.Stock}
		}
		result = append(result, r)
	}
	return result
}
func wishlistDTO(d *usecase.WishlistDetail) wishlistResponse {
	r := wishlistResponse{wishlistSummaryResponse: wishlistSummaryDTO(d.Wishlist), Items: wishlistItemsDTO(d.Items)}
	if d.SharePath != "" {
		r.SharePath = &d.SharePath
	}
	return r
}
func wishlistResult(c *gin.Context, d *usecase.WishlistDetail, err error, status int) {
	if err != nil {
		wishlistHTTPError(c, err)
		return
	}
	c.JSON(status, wishlistDTO(d))
}
func (h *Handler) listWishlists(c *gin.Context) {
	rows, err := h.wishlists.List(c.Request.Context(), actor(c))
	if err != nil {
		wishlistHTTPError(c, err)
		return
	}
	result := make([]wishlistSummaryResponse, 0, len(rows))
	for _, r := range rows {
		result = append(result, wishlistSummaryDTO(r))
	}
	c.JSON(200, result)
}

// RawMessage distinguishes missing, null and incorrectly typed text fields.
func wishlistTextField(c *gin.Context, raw json.RawMessage, required bool) (*string, bool) {
	if len(raw) == 0 {
		if !required {
			return nil, true
		}
	}
	var value *string
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil || value == nil {
		c.JSON(400, errorResponse{"wishlist text must be a string"})
		return nil, false
	}
	return value, true
}
func (h *Handler) createWishlist(c *gin.Context) {
	var r struct {
		Title       json.RawMessage `json:"title"`
		Description json.RawMessage `json:"description"`
	}
	if !decodeUserRequest(c, &r) {
		return
	}
	title, ok := wishlistTextField(c, r.Title, true)
	if !ok {
		return
	}
	desc, ok := wishlistTextField(c, r.Description, false)
	if !ok {
		return
	}
	description := ""
	if desc != nil {
		description = *desc
	}
	d, err := h.wishlists.Create(c.Request.Context(), actor(c), *title, description)
	wishlistResult(c, d, err, 201)
}
func (h *Handler) getWishlist(c *gin.Context) {
	id, ok := wishlistID(c, "id")
	if !ok {
		return
	}
	d, err := h.wishlists.Detail(c.Request.Context(), actor(c), id)
	wishlistResult(c, d, err, 200)
}
func (h *Handler) patchWishlist(c *gin.Context) {
	id, ok := wishlistID(c, "id")
	if !ok {
		return
	}
	var r struct {
		Title       json.RawMessage `json:"title"`
		Description json.RawMessage `json:"description"`
	}
	if !decodeUserRequest(c, &r) {
		return
	}
	title, ok := wishlistTextField(c, r.Title, false)
	if !ok {
		return
	}
	desc, ok := wishlistTextField(c, r.Description, false)
	if !ok {
		return
	}
	d, err := h.wishlists.Update(c.Request.Context(), actor(c), id, title, desc)
	wishlistResult(c, d, err, 200)
}
func (h *Handler) deleteWishlist(c *gin.Context) {
	id, ok := wishlistID(c, "id")
	if !ok || !withoutRequestBody(c) {
		return
	}
	if err := h.wishlists.Delete(c.Request.Context(), actor(c), id); err != nil {
		wishlistHTTPError(c, err)
		return
	}
	c.Status(204)
}
func (h *Handler) addWishlistItem(c *gin.Context) {
	id, ok := wishlistID(c, "id")
	if !ok {
		return
	}
	var r struct {
		ProductID int64  `json:"product_id"`
		VariantID *int64 `json:"product_variant_id"`
	}
	if !decodeUserRequest(c, &r) {
		return
	}
	d, err := h.wishlists.AddItem(c.Request.Context(), actor(c), id, r.ProductID, r.VariantID)
	wishlistResult(c, d, err, 201)
}
func (h *Handler) patchWishlistItem(c *gin.Context) {
	id, ok := wishlistID(c, "id")
	if !ok {
		return
	}
	item, ok := wishlistID(c, "item_id")
	if !ok {
		return
	}
	var r struct {
		Variant json.RawMessage `json:"product_variant_id"`
	}
	if !decodeUserRequest(c, &r) {
		return
	}
	var variant *int64
	if len(r.Variant) == 0 || json.Unmarshal(r.Variant, &variant) != nil {
		c.JSON(400, errorResponse{"product_variant_id must be an ID or null"})
		return
	}
	d, err := h.wishlists.ChangeItem(c.Request.Context(), actor(c), id, item, variant)
	wishlistResult(c, d, err, 200)
}
func (h *Handler) deleteWishlistItem(c *gin.Context) {
	id, ok := wishlistID(c, "id")
	if !ok {
		return
	}
	item, ok := wishlistID(c, "item_id")
	if !ok || !withoutRequestBody(c) {
		return
	}
	if err := h.wishlists.RemoveItem(c.Request.Context(), actor(c), id, item); err != nil {
		wishlistHTTPError(c, err)
		return
	}
	d, err := h.wishlists.Detail(c.Request.Context(), actor(c), id)
	wishlistResult(c, d, err, 200)
}
func (h *Handler) reorderWishlist(c *gin.Context) {
	id, ok := wishlistID(c, "id")
	if !ok {
		return
	}
	var r struct {
		IDs []int64 `json:"item_ids"`
	}
	if !decodeUserRequest(c, &r) {
		return
	}
	if r.IDs == nil {
		c.JSON(400, errorResponse{"item_ids must be an array"})
		return
	}
	d, err := h.wishlists.Reorder(c.Request.Context(), actor(c), id, r.IDs)
	wishlistResult(c, d, err, 200)
}
func (h *Handler) shareWishlist(c *gin.Context) {
	id, ok := wishlistID(c, "id")
	if !ok || !emptyAction(c) {
		return
	}
	d, err := h.wishlists.Share(c.Request.Context(), actor(c), id, true)
	wishlistResult(c, d, err, 200)
}
func (h *Handler) unshareWishlist(c *gin.Context) {
	id, ok := wishlistID(c, "id")
	if !ok || !withoutRequestBody(c) {
		return
	}
	d, err := h.wishlists.Share(c.Request.Context(), actor(c), id, false)
	wishlistResult(c, d, err, 200)
}
func (h *Handler) sharedWishlist(c *gin.Context) {
	d, err := h.wishlists.Shared(c.Request.Context(), c.Param("token"))
	if err != nil {
		wishlistHTTPError(c, err)
		return
	}
	// Deliberately separate from the owner DTO: no owner, share secret, private
	// profile fields, hidden products or counts of omitted products.
	c.JSON(200, struct {
		Title       string                 `json:"title"`
		Description string                 `json:"description"`
		UpdatedAt   time.Time              `json:"updated_at"`
		Items       []wishlistItemResponse `json:"items"`
	}{d.Wishlist.Title, d.Wishlist.Description, d.Wishlist.UpdatedAt, wishlistItemsDTO(d.Items)})
}
