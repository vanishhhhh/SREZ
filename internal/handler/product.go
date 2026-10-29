package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"srez/internal/entities"
)

func GetProducts(c *gin.Context) {
	products, err := mockProducts()
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "не удалось подготовить тестовые товары"})
		return
	}
	response := make([]productResponse, 0, len(products))
	for _, product := range products {
		response = append(response, productDTO(product))
	}
	c.JSON(http.StatusOK, response)
}

func GetProduct(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "id должен быть положительным целым числом int64"})
		return
	}
	products, err := mockProductCatalog()
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "не удалось подготовить тестовые товары"})
		return
	}
	for _, product := range products {
		if product.product.ID() == id {
			c.JSON(http.StatusOK, productDetailDTO(product.product, product.publication))
			return
		}
	}
	c.JSON(http.StatusNotFound, errorResponse{Error: "товар не найден"})
}

// CreateProduct validates a transient draft and returns a response example.
// The example ID belongs only to the DTO; nothing is saved or published.
func CreateProduct(c *gin.Context) {
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
	product, err := entities.NewProduct(request.BrandID, request.Name, request.Description,
		request.PriceKopecks, entities.ProductCategory(request.Category), request.Materials, time.Now().UTC())
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "некорректные данные товара"})
		return
	}
	response := productDTO(*product)
	response.ID = 3 // Fixed response example, absent from GET fixtures.
	c.JSON(http.StatusCreated, response)
}
