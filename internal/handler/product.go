package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"srez/internal/usecase"
)

func (h *Handler) GetProducts(c *gin.Context) {
	products, err := h.products.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "не удалось загрузить товары"})
		return
	}
	response := make([]productResponse, 0, len(products))
	for _, product := range products {
		response = append(response, productDTO(product))
	}
	c.JSON(http.StatusOK, response)
}

func (h *Handler) GetProduct(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "id должен быть положительным целым числом int64"})
		return
	}
	detail, err := h.products.Get(c.Request.Context(), id)
	if errors.Is(err, usecase.ErrProductNotFound) {
		c.JSON(http.StatusNotFound, errorResponse{Error: "товар не найден"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "не удалось загрузить товар"})
		return
	}
	c.JSON(http.StatusOK, productDetailDTO(detail.Product, detail.Publication))
}
