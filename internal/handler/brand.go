package handler

import (
	"errors"
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"

	"srez/internal/usecase"
)

// HTTP path format mirrors Brand.Slug; domain entities remain unchanged.
var brandSlugPattern = regexp.MustCompile(`^[a-z0-9-]{3,60}$`)

func (h *Handler) GetBrands(c *gin.Context) {
	brands, err := h.brands.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "не удалось загрузить бренды"})
		return
	}
	response := make([]brandResponse, 0, len(brands))
	for _, brand := range brands {
		response = append(response, brandDTO(brand))
	}
	c.JSON(http.StatusOK, response)
}

func (h *Handler) GetBrand(c *gin.Context) {
	slug := c.Param("slug")
	if !brandSlugPattern.MatchString(slug) {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "slug должен содержать 3–60 строчных латинских букв, цифр или дефисов"})
		return
	}
	brand, err := h.brands.Get(c.Request.Context(), slug)
	if errors.Is(err, usecase.ErrBrandNotFound) {
		c.JSON(http.StatusNotFound, errorResponse{Error: "бренд не найден"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "не удалось загрузить бренд"})
		return
	}
	c.JSON(http.StatusOK, brandDTO(*brand))
}
