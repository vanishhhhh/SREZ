package handler

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/gin-gonic/gin"

	"srez/internal/usecase"
)

func (h *Handler) GetProducts(c *gin.Context) {
	values, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "некорректные параметры каталога"})
		return
	}
	query, err := parseProductQuery(values)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "некорректные параметры каталога"})
		return
	}
	page, err := h.products.Browse(c.Request.Context(), query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "не удалось загрузить товары"})
		return
	}
	response := make([]productCardResponse, 0, len(page.Items))
	for _, product := range page.Items {
		response = append(response, productCardDTO(product))
	}
	totalPages := int64(0)
	if page.Total > 0 {
		totalPages = (page.Total-1)/int64(page.Limit) + 1
	}
	c.Header("X-Total-Count", strconv.FormatInt(page.Total, 10))
	c.Header("X-Page", strconv.FormatInt(page.Page, 10))
	c.Header("X-Limit", strconv.Itoa(page.Limit))
	c.Header("X-Total-Pages", strconv.FormatInt(totalPages, 10))
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
