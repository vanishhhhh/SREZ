package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"srez/internal/entities"
	"srez/internal/usecase"
)

// Test-only fixture for the pre-RBAC JSON contract. Production routes use Management.
func (h *Handler) fixtureCreateProduct(c *gin.Context) {
	body, ok := readJSONBody(c)
	if !ok {
		return
	}
	var request createProductRequest
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "нужен JSON-объект с полями товара"})
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "нужен ровно один JSON-объект"})
		return
	}
	product, err := h.products.Create(c.Request.Context(), request.BrandID, request.Name, request.Description,
		request.PriceKopecks, entities.ProductCategory(request.Category), request.Materials)
	if errors.Is(err, usecase.ErrBrandNotFound) {
		c.JSON(http.StatusNotFound, errorResponse{Error: "бренд не найден"})
		return
	}
	if errors.Is(err, usecase.ErrBrandInactive) {
		c.JSON(http.StatusConflict, errorResponse{Error: "бренд не активен"})
		return
	}
	if errors.Is(err, entities.ErrInvalidValue) {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "некорректные данные товара"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "не удалось сохранить товар"})
		return
	}
	c.JSON(http.StatusCreated, productDTO(*product))
}
