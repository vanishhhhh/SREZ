package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"srez/internal/usecase"
)

func (h *Handler) SetMedia(m *usecase.Media) { h.media = m }
func (h *Handler) mediaRoutes(x *authHTTP) {
	ready := func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if h.media == nil {
			c.AbortWithStatusJSON(503, errorResponse{"media unavailable"})
			return
		}
		c.Next()
	}
	h.router.POST("/media/uploads", x.bearer, ready, h.createMediaUpload)
	h.router.PUT("/media/uploads/:id/content", x.bearer, ready, h.uploadMediaContent)
	h.router.POST("/media/uploads/:id/complete", x.bearer, func(c *gin.Context) { c.JSON(410, errorResponse{"direct S3 uploads are retired"}) })
	optionalBearer := func(c *gin.Context) {
		if strings.TrimSpace(c.GetHeader("Authorization")) != "" {
			x.bearer(c)
		} else {
			c.Next()
		}
	}
	h.router.GET("/media/assets/:id/content", optionalBearer, ready, h.readMediaContent)
	h.router.DELETE("/profile/avatar", x.bearer, ready, func(c *gin.Context) { h.deleteMediaTarget(c, usecase.MediaTarget{Purpose: "user_avatar"}) })
	h.router.DELETE("/brands/:id/media/:purpose", x.bearer, ready, func(c *gin.Context) {
		id, ok := resourceID(c)
		if !ok {
			return
		}
		purpose := c.Param("purpose")
		if purpose != "brand_logo" && purpose != "brand_banner" {
			c.JSON(400, errorResponse{"invalid media purpose"})
			return
		}
		h.deleteMediaTarget(c, usecase.MediaTarget{Purpose: purpose, BrandID: id})
	})
}
func mediaHTTPError(c *gin.Context, e error) {
	switch {
	case errors.Is(e, usecase.ErrMediaFormat):
		c.JSON(400, errorResponse{"unsupported image format"})
	case errors.Is(e, usecase.ErrMediaResolution):
		c.JSON(400, errorResponse{"image resolution exceeds limit"})
	case errors.Is(e, usecase.ErrMediaCorrupt):
		c.JSON(400, errorResponse{"damaged image"})
	case errors.Is(e, usecase.ErrMediaLibraryFull):
		c.JSON(409, errorResponse{"editorial media library is full"})
	case errors.Is(e, usecase.ErrMediaTooLarge):
		c.JSON(413, errorResponse{"image size limit exceeded"})
	case errors.Is(e, usecase.ErrMediaBusy):
		c.Header("Retry-After", "5")
		c.JSON(429, errorResponse{"media processing busy"})
	default:
		managementHTTPError(c, e)
	}
}
func (h *Handler) createMediaUpload(c *gin.Context) {
	var r usecase.MediaRequest
	if !decodeUserRequest(c, &r) {
		return
	}
	u, e := h.media.Create(c.Request.Context(), actor(c), r)
	if e != nil {
		mediaHTTPError(c, e)
		return
	}
	c.JSON(201, gin.H{"upload": u, "content_url": "/media/uploads/" + strconv.FormatInt(u.ID, 10) + "/content"})
}
func (h *Handler) uploadMediaContent(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	if c.Request.ContentLength > usecase.MaxMediaBytes {
		c.JSON(413, errorResponse{"image size limit exceeded"})
		return
	}
	if c.Request.ContentLength < 0 || len(c.Request.TransferEncoding) > 0 {
		c.JSON(411, errorResponse{"Content-Length required"})
		return
	}
	defer c.Request.Body.Close()
	_ = http.NewResponseController(c.Writer).SetReadDeadline(time.Now().Add(10 * time.Second))
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, usecase.MaxMediaBytes)
	u, e := h.media.Upload(c.Request.Context(), actor(c), id, c.GetHeader("Content-Type"), c.Request.ContentLength, c.Request.Body)
	if e != nil {
		mediaHTTPError(c, e)
		return
	}
	c.JSON(200, u)
}
func (h *Handler) readMediaContent(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	var uid int64
	if v, ok := c.Get("auth_user_id"); ok {
		uid = v.(int64)
	}
	_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(15 * time.Second))
	defer func() { _ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Time{}) }()
	width := 0
	if values, exists := c.Request.URL.Query()["width"]; exists {
		if len(values) != 1 {
			c.JSON(400, errorResponse{"invalid media width"})
			return
		}
		width, _ = strconv.Atoi(values[0])
		if !usecase.ValidMediaWidth(width) {
			c.JSON(400, errorResponse{"invalid media width"})
			return
		}
	}
	e := h.media.DeliverSize(c.Request.Context(), uid, id, width, func(a *usecase.MediaAsset, data []byte) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Cache-Control", "no-store")
		c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
		c.Data(200, a.MIME, data)
	})
	switch {
	case errors.Is(e, usecase.ErrMediaBusy):
		c.Header("Retry-After", "1")
		c.JSON(http.StatusTooManyRequests, errorResponse{"media processing busy"})
	case errors.Is(e, usecase.ErrMediaStorageUnavailable):
		c.Header("Cache-Control", "no-store")
		c.Header("Retry-After", "1")
		c.JSON(http.StatusServiceUnavailable, errorResponse{"media temporarily unavailable"})
	case e != nil:
		mediaHTTPError(c, e)
	}
}
func (h *Handler) deleteMediaTarget(c *gin.Context, t usecase.MediaTarget) {
	if !withoutRequestBody(c) {
		return
	}
	if e := h.media.DeleteTarget(c.Request.Context(), actor(c), t); e != nil {
		mediaHTTPError(c, e)
		return
	}
	c.Status(204)
}
func mediaURL(legacy string, id int64) string {
	if id > 0 {
		return "/media/assets/" + strconv.FormatInt(id, 10) + "/content"
	}
	return legacy
}
