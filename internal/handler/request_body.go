package handler

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

const maxJSONBodyBytes = 64 * 1024

// Read before decoding so the limit applies to malformed JSON and trailing
// whitespace too, even when Content-Length is unknown.
func readJSONBody(c *gin.Context) ([]byte, bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxJSONBodyBytes)
	defer c.Request.Body.Close()
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, errorResponse{Error: "размер JSON-тела не должен превышать 64 КиБ"})
		} else {
			c.JSON(http.StatusBadRequest, errorResponse{Error: "не удалось прочитать JSON-тело"})
		}
		return nil, false
	}
	return body, true
}
