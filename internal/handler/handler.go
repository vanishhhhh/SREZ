package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	router *gin.Engine
}

func New() *Handler {
	router := gin.Default()
	router.GET("/projects", GetProjects)

	return &Handler{router: router}
}

func (h *Handler) Run(addr string) error {
	return h.router.Run(addr)
}

func GetProjects(c *gin.Context) {
	c.JSON(http.StatusOK, []gin.H{
		{"id": 1, "title": "Дело"},
		{"id": 2, "title": "Витрина"},
	})
}
