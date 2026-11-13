package handler

import (
	"errors"
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
	h.router.GET("/auth/csrf", x.csrfSource, x.csrf)
	h.router.POST("/auth/refresh", x.source, x.refresh)
	h.router.POST("/auth/logout", x.source, x.logout)
	h.router.GET("/profile", x.bearer, h.profile)
	h.router.PATCH("/profile", x.bearer, h.patchProfile)
	h.managementRoutes(x)
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
		c.JSON(400, errorResponse{"password must contain at least 12 characters and at most 1024 bytes"})
		return
	}
	u, e := x.auth.Register(c.Request.Context(), r.Email, r.Username, r.DisplayName, r.Password)
	if e != nil {
		authError(c, e)
		return
	}
	c.JSON(201, publicUserDTO(*u))
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
func (x *authHTTP) name() string {
	if x.cfg.LocalHTTP {
		return "srez_refresh_local"
	}
	return "__Host-srez_refresh"
}
func (x *authHTTP) cookie(c *gin.Context, token string, expires time.Time) {
	max := int(time.Until(expires).Seconds())
	if token == "" {
		max = -1
		expires = time.Unix(1, 0)
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: x.name(), Value: token, Path: "/", HttpOnly: true, Secure: !x.cfg.LocalHTTP, SameSite: http.SameSiteStrictMode, Expires: expires, MaxAge: max})
}
func (x *authHTTP) csrf(c *gin.Context) {
	refresh, _ := c.Cookie(x.name())
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
	refresh, _ := c.Cookie(x.name())
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
	id, e := x.tokens.Verify(parts[1])
	if e != nil {
		c.AbortWithStatusJSON(401, errorResponse{"invalid access token"})
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
	c.JSON(200, profileResponse{publicUserDTO(*u), u.Email()})
}
func (h *Handler) patchProfile(c *gin.Context) {
	var r updateUserRequest
	if !decodeUserRequest(c, &r) {
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
	c.JSON(200, profileResponse{publicUserDTO(*u), u.Email()})
}
