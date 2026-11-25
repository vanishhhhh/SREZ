package handler

import (
	"errors"
	"log"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"srez/internal/config"
	"srez/internal/entities"
	"srez/internal/security"
	"srez/internal/usecase"
)

type attempt struct {
	start time.Time
	count int
}
type authHTTP struct {
	auth     *usecase.Auth
	tokens   *security.Tokens
	cfg      config.Auth
	mu       sync.Mutex
	attempts map[string]attempt
}

func (h *Handler) EnableAuth(a *usecase.Auth, t *security.Tokens, cfg config.Auth) error {
	if err := h.router.SetTrustedProxies(cfg.Proxies); err != nil {
		return err
	}
	x := &authHTTP{auth: a, tokens: t, cfg: cfg, attempts: map[string]attempt{}}
	h.router.POST("/auth/register", x.source, x.limit, x.register)
	h.router.POST("/auth/login", x.source, x.limit, x.login)
	h.router.POST("/auth/verify-email", x.source, x.limit, x.verifyEmail)
	h.router.POST("/auth/resend-verification", x.source, x.limit, x.resendVerification)
	h.router.POST("/auth/forgot-password", x.source, x.limit, x.forgotPassword)
	h.router.POST("/auth/reset-password", x.source, x.limit, x.resetPassword)
	h.router.GET("/auth/csrf", x.csrfSource, x.csrf)
	h.router.POST("/auth/refresh", x.source, x.refresh)
	h.router.POST("/auth/logout", x.source, x.logout)
	h.router.GET("/profile", x.bearer, h.profile)
	h.router.PATCH("/profile", x.bearer, h.patchProfile)
	h.managementRoutes(x)
	h.shoppingRoutes(x)
	h.orderRoutes(x)
	h.mediaRoutes(x)
	h.wishlistRoutes(x)
	return nil
}
func (x *authHTTP) source(c *gin.Context) {
	x.checkSource(c, false)
}

// Browsers may omit Origin on same-origin GET. This exception is registered
// only for the read-only CSRF endpoint; POST keeps a mandatory exact Origin.
func (x *authHTTP) csrfSource(c *gin.Context) {
	x.checkSource(c, true)
}

func (x *authHTTP) checkSource(c *gin.Context, allowMissingOrigin bool) {
	c.Header("Cache-Control", "no-store")
	origin := c.GetHeader("Origin")
	if origin != x.cfg.Origin && !(allowMissingOrigin && origin == "") {
		c.AbortWithStatusJSON(403, errorResponse{"request origin denied"})
		return
	}
	if v := c.GetHeader("Sec-Fetch-Site"); v != "" && v != "same-origin" {
		c.AbortWithStatusJSON(403, errorResponse{"request source denied"})
		return
	}
	if c.Request.Method != "GET" {
		m, _, e := mime.ParseMediaType(c.ContentType())
		if e != nil || m != "application/json" {
			c.AbortWithStatusJSON(415, errorResponse{"application/json required"})
			return
		}
	}
	c.Next()
}
func (x *authHTTP) limit(c *gin.Context) {
	now := time.Now()
	x.mu.Lock()
	defer x.mu.Unlock()
	for key, v := range x.attempts {
		if now.Sub(v.start) >= time.Minute {
			delete(x.attempts, key)
		}
	}
	key := c.ClientIP()
	v := x.attempts[key]
	if len(x.attempts) >= 10000 && v.start.IsZero() {
		c.AbortWithStatusJSON(429, errorResponse{"try later"})
		return
	}
	if v.start.IsZero() {
		v.start = now
	}
	v.count++
	x.attempts[key] = v
	if v.count > x.cfg.Attempts {
		c.Header("Retry-After", "60")
		c.AbortWithStatusJSON(429, errorResponse{"try later"})
	}
}
func authError(c *gin.Context, err error) {
	status := 500
	message := "authentication operation failed"
	switch {
	case errors.Is(err, usecase.ErrResetToken):
		status, message = 400, "invalid or expired reset link"
	case errors.Is(err, usecase.ErrRecoveryRate):
		status, message = 429, "password recovery rate limited"
		c.Header("Retry-After", "60")
	case errors.Is(err, usecase.ErrRecoveryUnavailable):
		status, message = 503, "password recovery unavailable"
	case errors.Is(err, usecase.ErrEmailUnverified):
		status = 403
		message = "email verification required"
	case errors.Is(err, usecase.ErrVerificationToken):
		status = 400
		message = "invalid or expired verification link"
	case errors.Is(err, usecase.ErrVerificationRate):
		status = 429
		message = "email verification rate limited"
		c.Header("Retry-After", "60")
	case errors.Is(err, usecase.ErrVerificationUnavailable):
		status = 503
		message = "email verification unavailable"
	case errors.Is(err, usecase.ErrAuth), errors.Is(err, security.ErrInvalid):
		status = 401
		message = "invalid credentials"
	case errors.Is(err, entities.ErrInvalidValue):
		status = 400
		message = "invalid account data"
	case errors.Is(err, usecase.ErrAccountExists):
		status = 409
		message = "account unavailable"
	case errors.Is(err, usecase.ErrCSRF):
		status = 403
		message = "invalid csrf"
	case errors.Is(err, usecase.ErrRefreshRace):
		status = 409
		message = "refresh already rotated"
	case errors.Is(err, security.ErrBusy):
		status = 503
		message = "try later"
	}
	c.JSON(status, errorResponse{message})
}
func (x *authHTTP) register(c *gin.Context) {
	var r struct {
		Email       string `json:"email"`
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
	}
	if !decodeUserRequest(c, &r) {
		return
	}
	if !security.ValidPassword(r.Password) {
		c.JSON(400, errorResponse{"password must contain at least 8 characters and at most 1024 bytes"})
		return
	}
	u, e := x.auth.RegisterFrom(c.Request.Context(), r.Email, r.Username, r.DisplayName, r.Password, c.ClientIP())
	if e != nil && !(u != nil && errors.Is(e, usecase.ErrMailDelivery)) {
		authError(c, e)
		return
	}
	delivery := "sent"
	if e != nil {
		delivery = "unavailable"
		log.Print("verification_delivery_failed")
	}
	c.JSON(201, struct {
		publicUserResponse
		Required bool   `json:"email_verification_required"`
		Delivery string `json:"email_delivery_status"`
		Retry    int    `json:"resend_after_seconds"`
	}{publicUserDTO(*u), true, delivery, 60})
}
func (x *authHTTP) verifyEmail(c *gin.Context) {
	c.Header("Referrer-Policy", "no-referrer")
	var r struct {
		Token string `json:"token"`
	}
	if !decodeUserRequest(c, &r) {
		return
	}
	if e := x.auth.VerifyEmail(c.Request.Context(), r.Token); e != nil {
		authError(c, e)
		return
	}
	c.JSON(200, gin.H{"message": "email verified"})
}
func (x *authHTTP) resendVerification(c *gin.Context) {
	c.Header("Referrer-Policy", "no-referrer")
	var r struct {
		Identifier string `json:"identifier"`
	}
	if !decodeUserRequest(c, &r) {
		return
	}
	if e := x.auth.ResendVerification(c.Request.Context(), r.Identifier, c.ClientIP()); e != nil {
		authError(c, e)
		return
	}
	// Request accepted is deliberately NOT a claim that SMTP delivered a message.
	c.JSON(202, gin.H{"message": "If this account needs verification, a message will be attempted. Check your inbox or retry later.", "resend_after_seconds": 60})
}
func (x *authHTTP) login(c *gin.Context) {
	var r struct {
		Identifier string `json:"identifier"`
		Password   string `json:"password"`
	}
	if !decodeUserRequest(c, &r) {
		return
	}
	v, e := x.auth.Login(c.Request.Context(), r.Identifier, r.Password)
	if e != nil {
		authError(c, e)
		return
	}
	x.cookie(c, v.Refresh, v.ExpiresAt)
	c.JSON(200, v)
}
func (x *authHTTP) cookie(c *gin.Context, token string, expires time.Time) {
	max := int(time.Until(expires).Seconds())
	if token == "" {
		max = -1
		expires = time.Unix(1, 0)
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: x.cfg.RefreshCookieName(), Value: token, Path: "/", HttpOnly: true, Secure: !x.cfg.LocalHTTP, SameSite: http.SameSiteStrictMode, Expires: expires, MaxAge: max})
}
func (x *authHTTP) csrf(c *gin.Context) {
	refresh, _ := c.Cookie(x.cfg.RefreshCookieName())
	token, e := x.auth.CSRF(c.Request.Context(), refresh)
	if e != nil {
		authError(c, e)
		return
	}
	c.JSON(200, gin.H{"csrf_token": token})
}
func (x *authHTTP) refresh(c *gin.Context) { x.rotate(c, false) }
func (x *authHTTP) logout(c *gin.Context)  { x.rotate(c, true) }
func (x *authHTTP) rotate(c *gin.Context, logout bool) {
	var request struct{}
	if !decodeUserRequest(c, &request) {
		return
	}
	refresh, _ := c.Cookie(x.cfg.RefreshCookieName())
	result, e := x.auth.Refresh(c.Request.Context(), refresh, c.GetHeader("X-CSRF-Token"), logout)
	if e != nil {
		authError(c, e)
		return
	}
	if logout {
		x.cookie(c, "", time.Time{})
		c.Status(204)
		return
	}
	x.cookie(c, result.Refresh, result.ExpiresAt)
	c.JSON(200, result)
}
func (x *authHTTP) bearer(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	parts := strings.Fields(c.GetHeader("Authorization"))
	if len(parts) != 2 || parts[0] != "Bearer" {
		c.AbortWithStatusJSON(401, errorResponse{"bearer token required"})
		return
	}
	id, e := x.auth.Authenticate(c.Request.Context(), parts[1])
	if e != nil {
		if errors.Is(e, usecase.ErrAuth) {
			c.AbortWithStatusJSON(401, errorResponse{"invalid access token"})
		} else {
			c.AbortWithStatusJSON(503, errorResponse{"authentication unavailable"})
		}
		return
	}
	c.Set("auth_user_id", id)
	c.Next()
}
func (h *Handler) profile(c *gin.Context) {
	id := c.MustGet("auth_user_id").(int64)
	u, e := h.users.Get(c.Request.Context(), id)
	if e != nil {
		if errors.Is(e, usecase.ErrUserNotFound) {
			c.JSON(401, errorResponse{"user unavailable"})
		} else {
			authError(c, e)
		}
		return
	}
	c.JSON(200, profileDTO(*u))
}
func (h *Handler) patchProfile(c *gin.Context) {
	var r updateUserRequest
	if !decodeUserRequest(c, &r) {
		return
	}
	if r.AvatarURL != nil {
		c.JSON(400, errorResponse{"use media upload for avatars"})
		return
	}
	u, e := h.users.Update(c.Request.Context(), c.MustGet("auth_user_id").(int64), r.DisplayName, r.AvatarURL)
	if e != nil {
		if errors.Is(e, usecase.ErrEmptyUserUpdate) {
			c.JSON(400, errorResponse{"empty update"})
		} else {
			authError(c, e)
		}
		return
	}
	c.JSON(200, profileDTO(*u))
}
