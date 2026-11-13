package usecase

import (
	"context"
	"errors"
	"time"

	"srez/internal/entities"
)

var ErrResourceHidden = errors.New("resource not found")
var ErrForbidden = errors.New("operation forbidden")
var ErrConflict = errors.New("resource conflict")

type BrandPatch struct {
	Name, Slug, Description, LogoURL, BannerURL *string
	Contacts                                    *entities.BrandContacts
	Appearance                                  *entities.BrandAppearance
}
type ProductPatch struct {
	Name, Description *string
	PriceKopecks      *int64
	Category          *entities.ProductCategory
	Materials         *[]string
}
type ManagedBrand struct {
	Brand entities.Brand
	Role  entities.BrandRole
}
type ManagementTx interface {
	Membership(context.Context, int64) (*entities.BrandMembership, error)
	SaveMembership(context.Context, entities.BrandMembership) error
	FindUser(context.Context, string) (int64, error)
	Members(context.Context) ([]entities.BrandMembership, error)
	SaveBrand(context.Context, entities.Brand) error
	Products(context.Context) ([]entities.Product, error)
	Product(context.Context, int64) (*entities.Product, error)
	Publication(context.Context, int64) (entities.ProductPublication, error)
	SaveProduct(context.Context, entities.Product) error
	InsertProduct(context.Context, entities.Product) (*entities.Product, error)
}
type ManagementRepository interface {
	CreateBrand(context.Context, int64, entities.Brand) (*entities.Brand, error)
	MyBrands(context.Context, int64, bool) ([]ManagedBrand, error)
	// WithinBrand locks the brand before memberships/products and keeps the lock
	// through callback commit. Every management writer uses this same boundary.
	WithinBrand(context.Context, int64, func(ManagementTx, *entities.Brand) error) error
	ProductBrand(context.Context, int64) (int64, error)
}
type Management struct{ repo ManagementRepository }

func NewManagement(repo ManagementRepository) *Management { return &Management{repo} }
func (m *Management) CreateBrand(ctx context.Context, uid int64, name, slug string) (*entities.Brand, error) {
	if uid <= 0 {
		return nil, ErrForbidden
	}
	b, e := entities.NewBrand(name, slug, time.Now().UTC())
	if e != nil {
		return nil, e
	}
	return m.repo.CreateBrand(ctx, uid, *b)
}
func (m *Management) MyBrands(ctx context.Context, uid int64, invitations bool) ([]ManagedBrand, error) {
	if uid <= 0 {
		return nil, ErrForbidden
	}
	return m.repo.MyBrands(ctx, uid, invitations)
}
func authorize(ctx context.Context, tx ManagementTx, uid int64, owner bool) error {
	if uid <= 0 {
		return ErrForbidden
	}
	member, e := tx.Membership(ctx, uid)
	if e != nil {
		return e
	}
	if member == nil || member.Status() != entities.MembershipActive {
		return ErrResourceHidden
	}
	if member.Role() != entities.BrandOwner && member.Role() != entities.BrandEditor {
		return ErrResourceHidden
	}
	if owner && member.Role() != entities.BrandOwner {
		return ErrForbidden
	}
	return nil
}
func (m *Management) brand(ctx context.Context, uid, id int64, owner bool, fn func(ManagementTx, *entities.Brand) error) error {
	return m.repo.WithinBrand(ctx, id, func(tx ManagementTx, b *entities.Brand) error {
		if e := authorize(ctx, tx, uid, owner); e != nil {
			return e
		}
		return fn(tx, b)
	})
}
func (m *Management) GetBrand(ctx context.Context, uid, id int64) (*entities.Brand, error) {
	var result *entities.Brand
	e := m.brand(ctx, uid, id, false, func(_ ManagementTx, b *entities.Brand) error { result = b; return nil })
	return result, e
}
func (m *Management) ChangeBrand(ctx context.Context, uid, id int64, p BrandPatch, action string) (*entities.Brand, error) {
	var result *entities.Brand
	e := m.brand(ctx, uid, id, true, func(tx ManagementTx, b *entities.Brand) error {
		if b.Status() == entities.BrandSuspended {
			return ErrForbidden
		}
		now := time.Now().UTC()
		var e error
		switch action {
		case "publish":
			e = b.Publish(now)
		case "unpublish":
			e = b.Unpublish(now)
		case "patch":
			count := 0
			for _, v := range []struct {
				value  *string
				change func(string, time.Time) error
			}{{p.Name, b.ChangeName}, {p.Slug, b.ChangeSlug}, {p.Description, b.ChangeDescription}, {p.LogoURL, b.ChangeLogo}, {p.BannerURL, b.ChangeBanner}} {
				if v.value != nil {
					count++
					if e = v.change(*v.value, now); e != nil {
						return e
					}
				}
			}
			if p.Contacts != nil {
				count++
				if e = b.ChangeContacts(*p.Contacts, now); e != nil {
					return e
				}
			}
			if p.Appearance != nil {
				count++
				if e = b.ChangeAppearance(*p.Appearance, now); e != nil {
					return e
				}
			}
			if count == 0 {
				return entities.ErrInvalidValue
			}
		default:
			return entities.ErrInvalidValue
		}
		if e != nil {
			return e
		}
		if e = tx.SaveBrand(ctx, *b); e != nil {
			return e
		}
		result = b
		return nil
	})
	return result, e
}
func (m *Management) Members(ctx context.Context, uid, id int64) ([]entities.BrandMembership, error) {
	var result []entities.BrandMembership
	e := m.brand(ctx, uid, id, true, func(tx ManagementTx, _ *entities.Brand) error { var e error; result, e = tx.Members(ctx); return e })
	return result, e
}
func (m *Management) Invite(ctx context.Context, uid, id int64, username string) (*entities.BrandMembership, error) {
	var result *entities.BrandMembership
	e := m.brand(ctx, uid, id, true, func(tx ManagementTx, b *entities.Brand) error {
		target, e := tx.FindUser(ctx, username)
		if e != nil {
			return e
		}
		if target == 0 {
			return ErrResourceHidden
		}
		if target == uid {
			return ErrConflict
		}
		member, e := tx.Membership(ctx, target)
		if e != nil {
			return e
		}
		now := time.Now().UTC()
		if member == nil {
			member, e = entities.NewEditorInvitation(target, b.ID(), uid, now)
		} else {
			e = member.Reinvite(uid, now)
		}
		if e != nil {
			return e
		}
		if e = tx.SaveMembership(ctx, *member); e != nil {
			return e
		}
		result = member
		return nil
	})
	return result, e
}
func (m *Management) Invitation(ctx context.Context, uid, id int64, accept bool) (*entities.BrandMembership, error) {
	var result *entities.BrandMembership
	e := m.repo.WithinBrand(ctx, id, func(tx ManagementTx, _ *entities.Brand) error {
		if uid <= 0 {
			return ErrForbidden
		}
		member, e := tx.Membership(ctx, uid)
		if e != nil {
			return e
		}
		if member == nil {
			return ErrResourceHidden
		}
		if accept {
			e = member.Accept(time.Now().UTC())
		} else {
			e = member.Decline(time.Now().UTC())
		}
		if e != nil {
			return e
		}
		if e = tx.SaveMembership(ctx, *member); e != nil {
			return e
		}
		result = member
		return nil
	})
	return result, e
}
func (m *Management) Revoke(ctx context.Context, uid, id, target int64) error {
	return m.brand(ctx, uid, id, true, func(tx ManagementTx, _ *entities.Brand) error {
		member, e := tx.Membership(ctx, target)
		if e != nil {
			return e
		}
		if member == nil {
			return ErrResourceHidden
		}
		if member.Role() == entities.BrandOwner {
			return ErrForbidden
		}
		if e = member.Revoke(time.Now().UTC()); e != nil {
			return e
		}
		return tx.SaveMembership(ctx, *member)
	})
}
func (m *Management) CreateProduct(ctx context.Context, uid int64, p entities.Product) (*entities.Product, error) {
	var result *entities.Product
	e := m.brand(ctx, uid, p.BrandID(), false, func(tx ManagementTx, b *entities.Brand) error {
		if b.Status() == entities.BrandSuspended {
			return ErrForbidden
		}
		if p.ID() != 0 || p.Status() != entities.ProductDraft {
			return entities.ErrInvalidValue
		}
		var e error
		result, e = tx.InsertProduct(ctx, p)
		return e
	})
	return result, e
}
func (m *Management) Products(ctx context.Context, uid, id int64) ([]entities.Product, error) {
	var result []entities.Product
	e := m.brand(ctx, uid, id, false, func(tx ManagementTx, b *entities.Brand) error {
		if b.Status() == entities.BrandSuspended {
			return ErrForbidden
		}
		var e error
		result, e = tx.Products(ctx)
		return e
	})
	return result, e
}
func (m *Management) ChangeProduct(ctx context.Context, uid, id int64, p ProductPatch, action string) (*entities.Product, error) {
	brandID, e := m.repo.ProductBrand(ctx, id)
	if e != nil {
		return nil, e
	}
	var result *entities.Product
	e = m.brand(ctx, uid, brandID, false, func(tx ManagementTx, b *entities.Brand) error {
		if b.Status() == entities.BrandSuspended {
			return ErrForbidden
		}
		product, e := tx.Product(ctx, id)
		if e != nil {
			return e
		}
		if product == nil || product.BrandID() != b.ID() {
			return ErrResourceHidden
		}
		now := time.Now().UTC()
		switch action {
		case "publish":
			if b.Status() != entities.BrandActive {
				return ErrConflict
			}
			data, e := tx.Publication(ctx, id)
			if e != nil {
				return e
			}
			if e = product.Publish(data, now); e != nil {
				return e
			}
		case "hide":
			e = product.Hide(now)
		case "unpublish":
			e = product.Unpublish(now)
		case "patch":
			count := 0
			for _, v := range []struct {
				value  *string
				change func(string, time.Time) error
			}{{p.Name, product.ChangeName}, {p.Description, product.ChangeDescription}} {
				if v.value != nil {
					count++
					if e = v.change(*v.value, now); e != nil {
						return e
					}
				}
			}
			if p.PriceKopecks != nil {
				count++
				if e = product.ChangePrice(*p.PriceKopecks, now); e != nil {
					return e
				}
			}
			if p.Category != nil {
				count++
				if e = product.ChangeCategory(*p.Category, now); e != nil {
					return e
				}
			}
			if p.Materials != nil {
				count++
				if e = product.ChangeMaterials(*p.Materials, now); e != nil {
					return e
				}
			}
			if count == 0 {
				return entities.ErrInvalidValue
			}
		default:
			return entities.ErrInvalidValue
		}
		if e != nil {
			return e
		}
		if e = tx.SaveProduct(ctx, *product); e != nil {
			return e
		}
		result = product
		return nil
	})
	return result, e
}
