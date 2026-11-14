package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"srez/internal/entities"
	"srez/internal/usecase"
)

type managedColorResponse struct {
	ID        int64    `json:"id"`
	ProductID int64    `json:"product_id"`
	Color     string   `json:"color"`
	Images    []string `json:"images"`
}
type managedVariantResponse struct {
	ID             int64  `json:"id"`
	ProductColorID int64  `json:"product_color_id"`
	Size           string `json:"size"`
	Stock          int64  `json:"stock"`
	SKU            string `json:"sku"`
}
type managedImageResponse struct {
	ID             int64  `json:"id"`
	ProductColorID int64  `json:"product_color_id"`
	URL            string `json:"url"`
	Position       int    `json:"position"`
}
type colorWriteRequest struct {
	Color *string `json:"color"`
}
type variantWriteRequest struct {
	Size  *string `json:"size"`
	Stock *int64  `json:"stock"`
	SKU   *string `json:"sku"`
}
type imageWriteRequest struct {
	URL      *string `json:"url"`
	Position *int    `json:"position"`
}

func managedColorDTO(c entities.ProductColor) managedColorResponse {
	images := c.Images()
	if images == nil {
		images = []string{}
	}
	return managedColorResponse{c.ID(), c.ProductID(), c.Color(), images}
}
func managedVariantDTO(v entities.ProductVariant) managedVariantResponse {
	return managedVariantResponse{v.ID(), v.ProductColorID(), v.Size(), int64(v.Stock()), v.SKU()}
}
func managedImageDTO(i entities.ProductImage) managedImageResponse {
	return managedImageResponse{i.ID(), i.ProductColorID(), i.URL(), i.Position()}
}

func (h *Handler) variantRoutes(add func(string, string, gin.HandlerFunc)) {
	add("POST", "/products/:id/colors", h.createColor)
	add("GET", "/products/:id/colors", h.productColors)
	add("PATCH", "/products/:id/colors/:color_id", h.renameColor)
	add("DELETE", "/products/:id/colors/:color_id", h.deleteColor)
	add("POST", "/products/:id/colors/:color_id/variants", h.createVariant)
	add("GET", "/products/:id/colors/:color_id/variants", h.productVariants)
	add("PATCH", "/products/:id/colors/:color_id/variants/:variant_id", h.patchVariant)
	add("DELETE", "/products/:id/colors/:color_id/variants/:variant_id", h.deleteVariant)
	add("POST", "/products/:id/colors/:color_id/images", h.createImage)
	add("GET", "/products/:id/colors/:color_id/images", h.productImages)
	add("PATCH", "/products/:id/colors/:color_id/images/:image_id", h.patchImage)
	add("DELETE", "/products/:id/colors/:color_id/images/:image_id", h.deleteImage)
	add("POST", "/products/:id/colors/:color_id/images/reorder", h.reorderImages)
}

// Parse every part of the hierarchy before consulting storage. Belonging is
// checked from locked database state in the usecase, never inferred from IDs.
func variantIDs(c *gin.Context, children ...string) ([]int64, bool) {
	ids := make([]int64, 0, len(children)+1)
	for _, key := range append([]string{"id"}, children...) {
		id, err := strconv.ParseInt(c.Param(key), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(400, errorResponse{"invalid resource ID"})
			return nil, false
		}
		ids = append(ids, id)
	}
	return ids, true
}
func (h *Handler) productColors(c *gin.Context) {
	ids, ok := variantIDs(c)
	if !ok {
		return
	}
	colors, err := h.management.ProductColors(c.Request.Context(), actor(c), ids[0])
	if err != nil {
		managementHTTPError(c, err)
		return
	}
	response := make([]managedColorResponse, 0, len(colors))
	for _, color := range colors {
		response = append(response, managedColorDTO(color))
	}
	c.JSON(200, response)
}
func (h *Handler) createColor(c *gin.Context) {
	ids, ok := variantIDs(c)
	if !ok {
		return
	}
	var r colorWriteRequest
	if !decodeUserRequest(c, &r) {
		return
	}
	if r.Color == nil {
		managementHTTPError(c, entities.ErrInvalidValue)
		return
	}
	color, err := h.management.CreateColor(c.Request.Context(), actor(c), ids[0], *r.Color)
	if err != nil {
		managementHTTPError(c, err)
		return
	}
	c.JSON(201, managedColorDTO(*color))
}
func (h *Handler) renameColor(c *gin.Context) {
	ids, ok := variantIDs(c, "color_id")
	if !ok {
		return
	}
	var r colorWriteRequest
	if !decodeUserRequest(c, &r) {
		return
	}
	if r.Color == nil {
		managementHTTPError(c, entities.ErrInvalidValue)
		return
	}
	color, err := h.management.RenameColor(c.Request.Context(), actor(c), ids[0], ids[1], *r.Color)
	if err != nil {
		managementHTTPError(c, err)
		return
	}
	c.JSON(200, managedColorDTO(*color))
}
func (h *Handler) deleteColor(c *gin.Context) {
	ids, ok := variantIDs(c, "color_id")
	if !ok {
		return
	}
	if err := h.management.DeleteColor(c.Request.Context(), actor(c), ids[0], ids[1]); err != nil {
		managementHTTPError(c, err)
		return
	}
	c.Status(204)
}
func (h *Handler) productVariants(c *gin.Context) {
	ids, ok := variantIDs(c, "color_id")
	if !ok {
		return
	}
	variants, err := h.management.ProductVariants(c.Request.Context(), actor(c), ids[0], ids[1])
	if err != nil {
		managementHTTPError(c, err)
		return
	}
	response := make([]managedVariantResponse, 0, len(variants))
	for _, v := range variants {
		response = append(response, managedVariantDTO(v))
	}
	c.JSON(200, response)
}
func (h *Handler) createVariant(c *gin.Context) {
	ids, ok := variantIDs(c, "color_id")
	if !ok {
		return
	}
	var r variantWriteRequest
	if !decodeUserRequest(c, &r) {
		return
	}
	if r.Size == nil || r.Stock == nil {
		managementHTTPError(c, entities.ErrInvalidValue)
		return
	}
	sku := ""
	if r.SKU != nil {
		sku = *r.SKU
	}
	v, err := h.management.CreateVariant(c.Request.Context(), actor(c), ids[0], ids[1], *r.Size, *r.Stock, sku)
	if err != nil {
		managementHTTPError(c, err)
		return
	}
	c.JSON(201, managedVariantDTO(*v))
}
func (h *Handler) patchVariant(c *gin.Context) {
	ids, ok := variantIDs(c, "color_id", "variant_id")
	if !ok {
		return
	}
	var r variantWriteRequest
	if !decodeUserRequest(c, &r) {
		return
	}
	v, err := h.management.ChangeVariant(c.Request.Context(), actor(c), ids[0], ids[1], ids[2], usecase.VariantPatch{Size: r.Size, Stock: r.Stock, SKU: r.SKU})
	if err != nil {
		managementHTTPError(c, err)
		return
	}
	c.JSON(200, managedVariantDTO(*v))
}
func (h *Handler) deleteVariant(c *gin.Context) {
	ids, ok := variantIDs(c, "color_id", "variant_id")
	if !ok {
		return
	}
	if err := h.management.DeleteVariant(c.Request.Context(), actor(c), ids[0], ids[1], ids[2]); err != nil {
		managementHTTPError(c, err)
		return
	}
	c.Status(204)
}
func (h *Handler) productImages(c *gin.Context) {
	ids, ok := variantIDs(c, "color_id")
	if !ok {
		return
	}
	images, err := h.management.ProductImages(c.Request.Context(), actor(c), ids[0], ids[1])
	if err != nil {
		managementHTTPError(c, err)
		return
	}
	response := make([]managedImageResponse, 0, len(images))
	for _, i := range images {
		response = append(response, managedImageDTO(i))
	}
	c.JSON(200, response)
}
func (h *Handler) createImage(c *gin.Context) {
	ids, ok := variantIDs(c, "color_id")
	if !ok {
		return
	}
	var r imageWriteRequest
	if !decodeUserRequest(c, &r) {
		return
	}
	if r.URL == nil {
		managementHTTPError(c, entities.ErrInvalidValue)
		return
	}
	i, err := h.management.CreateImage(c.Request.Context(), actor(c), ids[0], ids[1], *r.URL, r.Position)
	if err != nil {
		managementHTTPError(c, err)
		return
	}
	c.JSON(201, managedImageDTO(*i))
}
func (h *Handler) patchImage(c *gin.Context) {
	ids, ok := variantIDs(c, "color_id", "image_id")
	if !ok {
		return
	}
	var r imageWriteRequest
	if !decodeUserRequest(c, &r) {
		return
	}
	i, err := h.management.ChangeImage(c.Request.Context(), actor(c), ids[0], ids[1], ids[2], usecase.ImagePatch{URL: r.URL, Position: r.Position})
	if err != nil {
		managementHTTPError(c, err)
		return
	}
	c.JSON(200, managedImageDTO(*i))
}
func (h *Handler) deleteImage(c *gin.Context) {
	ids, ok := variantIDs(c, "color_id", "image_id")
	if !ok {
		return
	}
	if err := h.management.DeleteImage(c.Request.Context(), actor(c), ids[0], ids[1], ids[2]); err != nil {
		managementHTTPError(c, err)
		return
	}
	c.Status(204)
}
func (h *Handler) reorderImages(c *gin.Context) {
	ids, ok := variantIDs(c, "color_id")
	if !ok {
		return
	}
	var r struct {
		ImageIDs []int64 `json:"image_ids"`
	}
	if !decodeUserRequest(c, &r) {
		return
	}
	if r.ImageIDs == nil || len(r.ImageIDs) > 10 {
		managementHTTPError(c, entities.ErrInvalidValue)
		return
	}
	images, err := h.management.ReorderImages(c.Request.Context(), actor(c), ids[0], ids[1], r.ImageIDs)
	if err != nil {
		managementHTTPError(c, err)
		return
	}
	response := make([]managedImageResponse, 0, len(images))
	for _, i := range images {
		response = append(response, managedImageDTO(i))
	}
	c.JSON(200, response)
}
