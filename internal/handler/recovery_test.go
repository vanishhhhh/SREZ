package handler

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"syscall"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSafeRecovery(t *testing.T) {
	oldMode, oldError, oldWriter := gin.Mode(), gin.DefaultErrorWriter, gin.DefaultWriter
	defer func() { gin.SetMode(oldMode); gin.DefaultErrorWriter = oldError; gin.DefaultWriter = oldWriter }()
	for _, mode := range []string{gin.DebugMode, gin.ReleaseMode} {
		for _, kind := range []string{"ordinary", "pipe", "reset", "abort"} {
			t.Run(mode+kind, func(t *testing.T) {
				var logs bytes.Buffer
				gin.SetMode(mode)
				gin.DefaultErrorWriter = &logs
				gin.DefaultWriter = &logs
				h := NewWithUsers(nil)
				fault := error(errors.New("PANIC_SECRET_SENTINEL"))
				broken := kind != "ordinary"
				switch kind {
				case "pipe":
					fault = fmt.Errorf("PANIC_SECRET_SENTINEL: %w", syscall.EPIPE)
				case "reset":
					fault = fmt.Errorf("PANIC_SECRET_SENTINEL: %w", syscall.ECONNRESET)
				case "abort":
					fault = http.ErrAbortHandler
				}
				h.router.POST("/fault", func(*gin.Context) { panic(fault) })
				req := httptest.NewRequest("POST", "/fault?token=QUERY_SECRET_SENTINEL", strings.NewReader("BODY_SECRET_SENTINEL"))
				req.Header.Set("Cookie", "refresh=COOKIE_SECRET_SENTINEL")
				req.Header.Set("X-CSRF-Token", "CSRF_SECRET_SENTINEL")
				req.Header.Set("Authorization", "Bearer ACCESS_SECRET_SENTINEL")
				w := httptest.NewRecorder()
				h.router.ServeHTTP(w, req)
				if !strings.Contains(logs.String(), "http_recovery kind=") {
					t.Fatal("missing diagnostic event")
				}
				for _, secret := range []string{"PANIC_SECRET_SENTINEL", "QUERY_SECRET_SENTINEL", "BODY_SECRET_SENTINEL", "COOKIE_SECRET_SENTINEL", "CSRF_SECRET_SENTINEL", "ACCESS_SECRET_SENTINEL"} {
					if strings.Contains(logs.String(), secret) || strings.Contains(w.Body.String(), secret) {
						t.Fatal("secret leaked")
					}
				}
				if broken {
					if w.Body.Len() != 0 {
						t.Fatal("wrote to broken connection")
					}
				} else {
					if w.Code != 500 {
						t.Fatal(w.Code)
					}
					assertCatalogJSON(t, w.Body.Bytes(), `{"error":"internal server error"}`)
				}
			})
		}
	}
}
func TestSafeRecoveryStringPanicAndStartedResponse(t *testing.T) {
	var logs bytes.Buffer
	r := gin.New()
	r.Use(safeRecovery(&logs))
	r.GET("/started", func(c *gin.Context) { c.String(202, "safe response"); panic("SECRET_PANIC") })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/started", nil))
	if w.Code != 202 || w.Body.String() != "safe response" || strings.Contains(logs.String(), "SECRET_PANIC") {
		t.Fatal("unsafe response after panic")
	}
}
