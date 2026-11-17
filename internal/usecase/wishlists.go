package usecase

import (
	"context"
	"errors"
	"srez/internal/entities"
)

var (
	ErrWishlistLimit     = errors.New("wishlist limit reached")
	ErrWishlistDuplicate = errors.New("product already in wishlist")
	ErrWishlistChanged   = errors.New("wishlist changed; reload before reordering")
)

type WishlistVariant struct {
	ID          int64
	Color, Size string
	Stock       int64
}
type WishlistItem struct {
	ID, ProductID int64
	VariantID     *int64
	Position      int
	Product       *ProductCard
	Variant       *WishlistVariant
}
type WishlistDetail struct {
	Wishlist  entities.Wishlist
	Items     []WishlistItem
	SharePath string
}
type WishlistRepository interface {
	List(context.Context, int64) ([]entities.Wishlist, error)
	Detail(context.Context, int64, int64) (*WishlistDetail, error)
	Shared(context.Context, []byte) (*WishlistDetail, error)
	Create(context.Context, int64, string, string) (int64, error)
	Update(context.Context, int64, int64, *string, *string) error
	Delete(context.Context, int64, int64) error
	AddItem(context.Context, int64, int64, int64, *int64) error
	ChangeItem(context.Context, int64, int64, int64, *int64) error
	RemoveItem(context.Context, int64, int64, int64) error
	Reorder(context.Context, int64, int64, []int64) error
	Share(context.Context, int64, int64, []byte, []byte) error
}
type WishlistLinkService interface {
	New() (string, []byte, []byte, error)
	Open([]byte) (string, error)
	Hash(string) ([]byte, error)
}
type Wishlists struct {
	repo  WishlistRepository
	links WishlistLinkService
}

func NewWishlists(repo WishlistRepository, links WishlistLinkService) *Wishlists {
	return &Wishlists{repo, links}
}
func wishlistIDs(uid, id int64) error {
	if uid <= 0 || id <= 0 {
		return entities.ErrInvalidValue
	}
	return nil
}
func (s *Wishlists) List(ctx context.Context, uid int64) ([]entities.Wishlist, error) {
	if uid <= 0 {
		return nil, entities.ErrInvalidValue
	}
	return s.repo.List(ctx, uid)
}
func (s *Wishlists) Detail(ctx context.Context, uid, id int64) (*WishlistDetail, error) {
	if err := wishlistIDs(uid, id); err != nil {
		return nil, err
	}
	d, err := s.repo.Detail(ctx, uid, id)
	if err != nil {
		return nil, err
	}
	if d == nil {
		return nil, ErrResourceHidden
	}
	if d.Wishlist.Visibility == "public" {
		token, err := s.links.Open(d.Wishlist.ShareTokenSealed)
		if err != nil {
			return nil, err
		}
		d.SharePath = "/w/" + token
	}
	return d, nil
}
func (s *Wishlists) Shared(ctx context.Context, token string) (*WishlistDetail, error) {
	hash, err := s.links.Hash(token)
	if err != nil {
		return nil, ErrResourceHidden
	}
	d, err := s.repo.Shared(ctx, hash)
	if err != nil {
		return nil, err
	}
	if d == nil {
		return nil, ErrResourceHidden
	}
	return d, nil
}
func (s *Wishlists) Create(ctx context.Context, uid int64, title, description string) (*WishlistDetail, error) {
	if uid <= 0 {
		return nil, entities.ErrInvalidValue
	}
	title, description, err := entities.WishlistText(title, description)
	if err != nil {
		return nil, err
	}
	id, err := s.repo.Create(ctx, uid, title, description)
	if err != nil {
		return nil, err
	}
	return s.Detail(ctx, uid, id)
}
func (s *Wishlists) Update(ctx context.Context, uid, id int64, title, description *string) (*WishlistDetail, error) {
	if err := wishlistIDs(uid, id); err != nil {
		return nil, err
	}
	if title == nil && description == nil {
		return nil, entities.ErrInvalidValue
	}
	if title != nil {
		v, _, err := entities.WishlistText(*title, "")
		if err != nil {
			return nil, err
		}
		title = &v
	}
	if description != nil {
		_, v, err := entities.WishlistText("Wishlist", *description)
		if err != nil {
			return nil, err
		}
		description = &v
	}
	if err := s.repo.Update(ctx, uid, id, title, description); err != nil {
		return nil, err
	}
	return s.Detail(ctx, uid, id)
}
func (s *Wishlists) Delete(ctx context.Context, uid, id int64) error {
	if err := wishlistIDs(uid, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, uid, id)
}
func (s *Wishlists) AddItem(ctx context.Context, uid, id, product int64, variant *int64) (*WishlistDetail, error) {
	if err := wishlistIDs(uid, id); err != nil {
		return nil, err
	}
	if err := entities.ValidateWishlistItem(product, variant); err != nil {
		return nil, err
	}
	if err := s.repo.AddItem(ctx, uid, id, product, variant); err != nil {
		return nil, err
	}
	return s.Detail(ctx, uid, id)
}
func (s *Wishlists) ChangeItem(ctx context.Context, uid, id, item int64, variant *int64) (*WishlistDetail, error) {
	if err := wishlistIDs(uid, id); err != nil {
		return nil, err
	}
	if err := entities.ValidateWishlistItem(item, variant); err != nil {
		return nil, err
	}
	if err := s.repo.ChangeItem(ctx, uid, id, item, variant); err != nil {
		return nil, err
	}
	return s.Detail(ctx, uid, id)
}
func (s *Wishlists) RemoveItem(ctx context.Context, uid, id, item int64) error {
	if err := wishlistIDs(uid, id); err != nil {
		return err
	}
	if item <= 0 {
		return entities.ErrInvalidValue
	}
	return s.repo.RemoveItem(ctx, uid, id, item)
}
func (s *Wishlists) Reorder(ctx context.Context, uid, id int64, items []int64) (*WishlistDetail, error) {
	if err := wishlistIDs(uid, id); err != nil {
		return nil, err
	}
	if err := entities.ValidateWishlistOrder(items); err != nil {
		return nil, err
	}
	if err := s.repo.Reorder(ctx, uid, id, items); err != nil {
		return nil, err
	}
	return s.Detail(ctx, uid, id)
}
func (s *Wishlists) Share(ctx context.Context, uid, id int64, enabled bool) (*WishlistDetail, error) {
	if err := wishlistIDs(uid, id); err != nil {
		return nil, err
	}
	var hash, sealed []byte
	if enabled {
		_, h, c, err := s.links.New()
		if err != nil {
			return nil, err
		}
		hash, sealed = h, c
	}
	if err := s.repo.Share(ctx, uid, id, hash, sealed); err != nil {
		return nil, err
	}
	return s.Detail(ctx, uid, id)
}
