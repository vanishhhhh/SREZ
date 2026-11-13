package handler

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/gin-gonic/gin"
)

// safeRecovery never formats the panic value or request. Error strings and
// request dumps may themselves contain credentials. Frames contain no arguments.
func safeRecovery(out io.Writer) gin.HandlerFunc {
	logger := log.New(out, "", log.LstdFlags)
	return func(c *gin.Context) {
		defer func() {
			if value := recover(); value != nil {
				broken := false
				if err, ok := value.(error); ok {
					broken = errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, http.ErrAbortHandler)
				}
				kind := "panic"
				if broken {
					kind = "broken_connection"
				}
				pcs := make([]uintptr, 32)
				n := runtime.Callers(3, pcs)
				frames := runtime.CallersFrames(pcs[:n])
				var stack strings.Builder
				for {
					frame, more := frames.Next()
					fmt.Fprintf(&stack, "%s %s:%d\n", frame.Function, filepath.Base(frame.File), frame.Line)
					if !more {
						break
					}
				}
				logger.Printf("http_recovery kind=%s\n%s", kind, stack.String())
				c.Abort()
				if !broken && !c.Writer.Written() {
					c.JSON(http.StatusInternalServerError, errorResponse{"internal server error"})
				}
			}
		}()
		c.Next()
	}
}

// Access logs omit raw URL/query/header/body data as well as panic values.
func safeAccessLog(p gin.LogFormatterParams) string {
	return fmt.Sprintf("[HTTP] status=%d latency=%s\n", p.StatusCode, p.Latency)
}
