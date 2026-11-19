package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"srez/internal/entities"
	"srez/internal/usecase"
)

const maxBrandPageBodyBytes = 128 * 1024

func decodeBrandPage(c *gin.Context, target any) bool {
	body, ok := readBoundedJSONBody(c, maxBrandPageBodyBytes)
	if !ok {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if checkBrandPageJSON(body, target) != nil || d.Decode(target) != nil || d.Decode(new(any)) != io.EOF {
		c.JSON(http.StatusBadRequest, errorResponse{"invalid brand page configuration"})
		return false
	}
	return true
}

func brandPageResult(c *gin.Context, state entities.BrandPageEditorState, err error) {
	if err != nil {
		if errors.Is(err, usecase.ErrBrandPageWritesLimited) {
			c.JSON(http.StatusServiceUnavailable, errorResponse{"brand page version is read-only during rollout"})
			return
		}
		managementHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, state)
}

func (h *Handler) getBrandPage(c *gin.Context) {
	id, ok := resourceID(c)
	if ok {
		state, err := h.management.EditBrandPage(c.Request.Context(), actor(c), id)
		brandPageResult(c, state, err)
	}
}

type brandPageSaveConfig struct {
	SchemaVersion int                       `json:"schema_version"`
	Blocks        []entities.BrandPageBlock `json:"blocks"`
	Appearance    *struct {
		Accent     *string `json:"accent_color"`
		Background *string `json:"background_color"`
		Text       *string `json:"text_color"`
	} `json:"appearance"`
}

func (h *Handler) saveBrandPage(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	var request struct {
		Revision *int64              `json:"revision"`
		Config   brandPageSaveConfig `json:"config"`
	}
	if !decodeBrandPage(c, &request) || !brandPageRevision(c, request.Revision) {
		return
	}
	a := request.Config.Appearance
	if a == nil || a.Accent == nil || a.Background == nil || a.Text == nil {
		c.JSON(http.StatusBadRequest, errorResponse{"brand page appearance requires three color strings"})
		return
	}
	config := entities.BrandPageConfig{
		SchemaVersion: request.Config.SchemaVersion,
		Blocks:        request.Config.Blocks,
		Appearance:    entities.BrandAppearance{AccentColor: *a.Accent, BackgroundColor: *a.Background, TextColor: *a.Text},
	}
	state, err := h.management.SaveBrandPage(c.Request.Context(), actor(c), id, *request.Revision, config)
	brandPageResult(c, state, err)
}

func (h *Handler) publishBrandPage(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	var request struct {
		Revision *int64 `json:"revision"`
	}
	if !decodeBrandPage(c, &request) || !brandPageRevision(c, request.Revision) {
		return
	}
	state, err := h.management.PublishBrandPage(c.Request.Context(), actor(c), id, *request.Revision)
	brandPageResult(c, state, err)
}

func brandPageRevision(c *gin.Context, revision *int64) bool {
	if revision == nil || *revision < 0 || *revision > entities.MaxBrandPageRevision {
		c.JSON(http.StatusBadRequest, errorResponse{"invalid brand page revision"})
		return false
	}
	return true
}
