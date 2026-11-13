package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"srez/internal/entities"
	"srez/internal/usecase"
)

func (h *Handler) SetManagement(m *usecase.Management) { h.management = m }
func (h *Handler) managementReady(c *gin.Context) {
	if h.management == nil {
		c.AbortWithStatusJSON(503, errorResponse{"management unavailable"})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Next()
}
func (h *Handler) managementRoutes(x *authHTTP) {
	add := func(method, path string, fn gin.HandlerFunc) {
		h.router.Handle(method, path, x.bearer, h.managementReady, fn)
	}
	add("POST", "/brands", h.createBrand)
	add("GET", "/me/brands", h.myBrands)
	add("GET", "/me/brand-invitations", h.myInvitations)
	// Gin requires identical wildcard names within the GET /brands tree.
	// These private paths parse :slug as a numeric brand ID.
	add("GET", "/brands/:slug/manage", h.manageBrand)
	add("GET", "/brands/:slug/members", h.members)
	add("GET", "/brands/:slug/products/manage", h.manageProducts)
	add("PATCH", "/brands/:id", h.patchBrand)
	add("POST", "/brands/:id/publish", h.publishBrand)
	add("POST", "/brands/:id/unpublish", h.unpublishBrand)
	add("POST", "/brands/:id/invitations", h.invite)
	add("POST", "/brands/:id/invitations/accept", h.acceptInvitation)
	add("POST", "/brands/:id/invitations/decline", h.declineInvitation)
	add("DELETE", "/brands/:id/members/:user_id", h.revoke)
	add("POST", "/products", h.CreateProduct)
	add("PATCH", "/products/:id", h.patchProduct)
	add("POST", "/products/:id/publish", h.publishProduct)
	add("POST", "/products/:id/hide", h.hideProduct)
	add("POST", "/products/:id/unpublish", h.unpublishProduct)
}
func actor(c *gin.Context) int64 { return c.MustGet("auth_user_id").(int64) }
func resourceID(c *gin.Context) (int64, bool) {
	value := c.Param("id")
	if value == "" {
		value = c.Param("slug")
	}
	id, e := strconv.ParseInt(value, 10, 64)
	if e != nil || id <= 0 {
		c.JSON(400, errorResponse{"invalid resource ID"})
		return 0, false
	}
	return id, true
}
func managementHTTPError(c *gin.Context, e error) {
	status, message := 500, "management operation failed"
	switch {
	case errors.Is(e, usecase.ErrResourceHidden):
		status, message = 404, "resource not found"
	case errors.Is(e, usecase.ErrForbidden):
		status, message = 403, "operation forbidden"
	case errors.Is(e, usecase.ErrConflict), errors.Is(e, entities.ErrInvalidTransition):
		status, message = 409, "state conflict"
	case errors.Is(e, entities.ErrNotReady):
		status, message = 409, "resource is not ready for publication"
	case errors.Is(e, entities.ErrInvalidValue):
		status, message = 400, "invalid resource data"
	}
	c.JSON(status, errorResponse{message})
}
func emptyAction(c *gin.Context) bool { var r struct{}; return decodeUserRequest(c, &r) }
func (h *Handler) createBrand(c *gin.Context) {
	var r struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if !decodeUserRequest(c, &r) {
		return
	}
	b, e := h.management.CreateBrand(c.Request.Context(), actor(c), r.Name, r.Slug)
	if e != nil {
		managementHTTPError(c, e)
		return
	}
	c.JSON(201, brandDTO(*b))
}
func (h *Handler) myBrands(c *gin.Context)      { h.listMine(c, false) }
func (h *Handler) myInvitations(c *gin.Context) { h.listMine(c, true) }
func (h *Handler) listMine(c *gin.Context, invitations bool) {
	brands, e := h.management.MyBrands(c.Request.Context(), actor(c), invitations)
	if e != nil {
		managementHTTPError(c, e)
		return
	}
	type item struct {
		Brand brandResponse `json:"brand"`
		Role  string        `json:"role"`
	}
	result := make([]item, 0, len(brands))
	for _, b := range brands {
		result = append(result, item{brandDTO(b.Brand), string(b.Role)})
	}
	c.JSON(200, result)
}
func (h *Handler) manageBrand(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	b, e := h.management.GetBrand(c.Request.Context(), actor(c), id)
	if e != nil {
		managementHTTPError(c, e)
		return
	}
	c.JSON(200, brandDTO(*b))
}

type brandPatchRequest struct {
	Name        *string                  `json:"name"`
	Slug        *string                  `json:"slug"`
	Description *string                  `json:"description"`
	LogoURL     *string                  `json:"logo_url"`
	BannerURL   *string                  `json:"banner_url"`
	Contacts    *brandContactsResponse   `json:"contacts"`
	Appearance  *brandAppearanceResponse `json:"appearance"`
}

func (h *Handler) patchBrand(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	var r brandPatchRequest
	if !decodeUserRequest(c, &r) {
		return
	}
	p := usecase.BrandPatch{Name: r.Name, Slug: r.Slug, Description: r.Description, LogoURL: r.LogoURL, BannerURL: r.BannerURL}
	if r.Contacts != nil {
		p.Contacts = &entities.BrandContacts{Email: r.Contacts.Email, TelegramURL: r.Contacts.TelegramURL, WebsiteURL: r.Contacts.WebsiteURL}
	}
	if r.Appearance != nil {
		p.Appearance = &entities.BrandAppearance{AccentColor: r.Appearance.AccentColor, BackgroundColor: r.Appearance.BackgroundColor, TextColor: r.Appearance.TextColor}
	}
	b, e := h.management.ChangeBrand(c.Request.Context(), actor(c), id, p, "patch")
	if e != nil {
		managementHTTPError(c, e)
		return
	}
	c.JSON(200, brandDTO(*b))
}
func (h *Handler) publishBrand(c *gin.Context)   { h.brandAction(c, "publish") }
func (h *Handler) unpublishBrand(c *gin.Context) { h.brandAction(c, "unpublish") }
func (h *Handler) brandAction(c *gin.Context, action string) {
	id, ok := resourceID(c)
	if !ok || !emptyAction(c) {
		return
	}
	b, e := h.management.ChangeBrand(c.Request.Context(), actor(c), id, usecase.BrandPatch{}, action)
	if e != nil {
		managementHTTPError(c, e)
		return
	}
	c.JSON(200, brandDTO(*b))
}

type membershipResponse struct {
	UserID    int64      `json:"user_id"`
	BrandID   int64      `json:"brand_id"`
	Role      string     `json:"role"`
	Status    string     `json:"status"`
	InvitedBy *int64     `json:"invited_by_user_id"`
	InvitedAt *time.Time `json:"invited_at"`
	JoinedAt  *time.Time `json:"joined_at"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

func memberDTO(m entities.BrandMembership) membershipResponse {
	return membershipResponse{m.UserID(), m.BrandID(), string(m.Role()), string(m.Status()), m.InvitedByUserID(), m.InvitedAt(), m.JoinedAt(), m.CreatedAt(), m.UpdatedAt()}
}
func (h *Handler) members(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	members, e := h.management.Members(c.Request.Context(), actor(c), id)
	if e != nil {
		managementHTTPError(c, e)
		return
	}
	result := make([]membershipResponse, 0, len(members))
	for _, m := range members {
		result = append(result, memberDTO(m))
	}
	c.JSON(200, result)
}
func (h *Handler) invite(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	var r struct {
		Username string `json:"username"`
	}
	if !decodeUserRequest(c, &r) {
		return
	}
	m, e := h.management.Invite(c.Request.Context(), actor(c), id, r.Username)
	if e != nil {
		managementHTTPError(c, e)
		return
	}
	c.JSON(201, memberDTO(*m))
}
func (h *Handler) acceptInvitation(c *gin.Context)  { h.invitationAction(c, true) }
func (h *Handler) declineInvitation(c *gin.Context) { h.invitationAction(c, false) }
func (h *Handler) invitationAction(c *gin.Context, accept bool) {
	id, ok := resourceID(c)
	if !ok || !emptyAction(c) {
		return
	}
	m, e := h.management.Invitation(c.Request.Context(), actor(c), id, accept)
	if e != nil {
		managementHTTPError(c, e)
		return
	}
	c.JSON(200, memberDTO(*m))
}
func (h *Handler) revoke(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	target, e := strconv.ParseInt(c.Param("user_id"), 10, 64)
	if e != nil || target <= 0 {
		c.JSON(400, errorResponse{"invalid user ID"})
		return
	}
	if e = h.management.Revoke(c.Request.Context(), actor(c), id, target); e != nil {
		managementHTTPError(c, e)
		return
	}
	c.Status(204)
}
func (h *Handler) CreateProduct(c *gin.Context) {
	var r createProductRequest
	if !decodeUserRequest(c, &r) {
		return
	}
	p, e := entities.NewProduct(r.BrandID, r.Name, r.Description, r.PriceKopecks, entities.ProductCategory(r.Category), r.Materials, time.Now().UTC())
	if e != nil {
		managementHTTPError(c, e)
		return
	}
	saved, e := h.management.CreateProduct(c.Request.Context(), actor(c), *p)
	if e != nil {
		managementHTTPError(c, e)
		return
	}
	c.JSON(201, productDTO(*saved))
}
func (h *Handler) manageProducts(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	products, e := h.management.Products(c.Request.Context(), actor(c), id)
	if e != nil {
		managementHTTPError(c, e)
		return
	}
	result := make([]productResponse, 0, len(products))
	for _, p := range products {
		result = append(result, productDTO(p))
	}
	c.JSON(200, result)
}

type productPatchRequest struct {
	Name         *string                   `json:"name"`
	Description  *string                   `json:"description"`
	PriceKopecks *int64                    `json:"price_kopecks"`
	Category     *entities.ProductCategory `json:"category"`
	Materials    *[]string                 `json:"materials"`
}

func (h *Handler) patchProduct(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	var r productPatchRequest
	if !decodeUserRequest(c, &r) {
		return
	}
	p, e := h.management.ChangeProduct(c.Request.Context(), actor(c), id, usecase.ProductPatch{Name: r.Name, Description: r.Description, PriceKopecks: r.PriceKopecks, Category: r.Category, Materials: r.Materials}, "patch")
	if e != nil {
		managementHTTPError(c, e)
		return
	}
	c.JSON(200, productDTO(*p))
}
func (h *Handler) publishProduct(c *gin.Context)   { h.productAction(c, "publish") }
func (h *Handler) hideProduct(c *gin.Context)      { h.productAction(c, "hide") }
func (h *Handler) unpublishProduct(c *gin.Context) { h.productAction(c, "unpublish") }
func (h *Handler) productAction(c *gin.Context, action string) {
	id, ok := resourceID(c)
	if !ok || !emptyAction(c) {
		return
	}
	p, e := h.management.ChangeProduct(c.Request.Context(), actor(c), id, usecase.ProductPatch{}, action)
	if e != nil {
		managementHTTPError(c, e)
		return
	}
	c.JSON(http.StatusOK, productDTO(*p))
}
