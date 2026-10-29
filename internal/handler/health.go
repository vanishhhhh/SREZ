package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetHealth reports only that this HTTP process can serve a request.
func GetHealth(c *gin.Context) {
	c.JSON(http.StatusOK, healthResponse{Status: "ok"})
}
