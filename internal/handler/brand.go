package handler

import (
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"
)

// HTTP path format mirrors Brand.Slug; domain entities remain unchanged.
var brandSlugPattern = regexp.MustCompile(`^[a-z0-9-]{3,60}$`)

func GetBrands(c *gin.Context) {
	brands, err := mockBrands()
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "не удалось подготовить тестовые бренды"})
		return
	}
	response := make([]brandResponse, 0, len(brands))
	for _, brand := range brands {
		response = append(response, brandDTO(brand))
	}
	c.JSON(http.StatusOK, response)
}

func GetBrand(c *gin.Context) {
	slug := c.Param("slug")
	if !brandSlugPattern.MatchString(slug) {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "slug должен содержать 3–60 строчных латинских букв, цифр или дефисов"})
		return
	}
	brands, err := mockBrands()
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "не удалось подготовить тестовые бренды"})
		return
	}
	for _, brand := range brands {
		if brand.Slug() == slug {
			c.JSON(http.StatusOK, brandDTO(brand))
			return
		}
	}
	c.JSON(http.StatusNotFound, errorResponse{Error: "бренд не найден"})
}
