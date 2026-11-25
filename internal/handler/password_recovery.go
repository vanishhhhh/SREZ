package handler

import (
	"github.com/gin-gonic/gin"
	"srez/internal/security"
)

func (x *authHTTP) forgotPassword(c *gin.Context) {
	c.Header("Referrer-Policy", "no-referrer")
	var r struct {
		Email string `json:"email"`
	}
	if !decodeUserRequest(c, &r) {
		return
	}
	if err := x.auth.ForgotPassword(c.Request.Context(), r.Email, c.ClientIP()); err != nil {
		authError(c, err)
		return
	}
	c.JSON(202, gin.H{"message": "If this address is eligible for recovery, a message will be attempted. Check your inbox or retry later.", "resend_after_seconds": 60})
}
func (x *authHTTP) resetPassword(c *gin.Context) {
	c.Header("Referrer-Policy", "no-referrer")
	var r struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !decodeUserRequest(c, &r) {
		return
	}
	if !security.ValidPassword(r.Password) {
		c.JSON(400, errorResponse{"password must contain at least 8 characters and at most 1024 bytes"})
		return
	}
	if err := x.auth.ResetPassword(c.Request.Context(), r.Token, r.Password); err != nil {
		authError(c, err)
		return
	}
	// Reset does not authenticate the browser or change another account's cookie.
	c.JSON(200, gin.H{"message": "password reset; sign in again"})
}
