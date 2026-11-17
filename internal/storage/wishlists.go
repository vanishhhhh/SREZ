package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"srez/internal/entities"
	"srez/internal/usecase"
)

type Wishlists struct{ db *sqlx.DB }

func NewWishlists(db *sqlx.DB) *Wishlists { return &Wishlists{db} }

var _ usecase.WishlistRepository = (*Wishlists)(nil)

type wishlistRow struct {
	ID          int64     `db:"id"`
	OwnerID     int64     `db:"owner_user_id"`
	Title       string    `db:"title"`
	Description string    `db:"description"`
	Visibility  string    `db:"visibility"`
	Hash        []byte    `db:"share_token_hash"`
	Sealed      []byte    `db:"share_token_sealed"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
	ItemCount   int       `db:"item_count"`
}

const wishlistColumns = `id,owner_user_id,title,description,visibility,share_token_hash,share_token_sealed,created_at,updated_at`

func (r wishlistRow) entity() entities.Wishlist {
	return entities.Wishlist{ID: r.ID, OwnerUserID: r.OwnerID, Title: r.Title, Description: r.Description, Visibility: r.Visibility, ShareTokenHash: r.Hash, ShareTokenSealed: r.Sealed, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, ItemCount: r.ItemCount}
}
func (s *Wishlists) List(ctx context.Context, uid int64) ([]entities.Wishlist, error) {
	var rows []wishlistRow
	err := s.db.SelectContext(ctx, &rows, `SELECT w.id,w.owner_user_id,w.title,w.description,w.visibility,w.created_at,w.updated_at,count(i.id) AS item_count
 FROM wishlists w LEFT JOIN wishlist_items i ON i.wishlist_id=w.id WHERE w.owner_user_id=$1
 GROUP BY w.id ORDER BY w.updated_at DESC,w.id DESC`, uid)
	if err != nil {
		return nil, err
	}
	result := make([]entities.Wishlist, 0, len(rows))
	for _, r := range rows {
		result = append(result, r.entity())
	}
	return result, nil
}
func (s *Wishlists) Detail(ctx context.Context, uid, id int64) (*usecase.WishlistDetail, error) {
	return s.read(ctx, uid, id, nil)
}
func (s *Wishlists) Shared(ctx context.Context, hash []byte) (*usecase.WishlistDetail, error) {
	return s.read(ctx, 0, 0, hash)
}
func (s *Wishlists) read(ctx context.Context, uid, id int64, hash []byte) (*usecase.WishlistDetail, error) {
	tx, err := s.db.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var meta wishlistRow
	if hash == nil {
		err = tx.GetContext(ctx, &meta, `SELECT `+wishlistColumns+` FROM wishlists WHERE id=$1 AND owner_user_id=$2`, id, uid)
	} else {
		err = tx.GetContext(ctx, &meta, `SELECT `+wishlistColumns+` FROM wishlists WHERE visibility='public' AND share_token_hash=$1`, hash)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, usecase.ErrResourceHidden
	}
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ID        int64  `db:"id"`
		ProductID int64  `db:"product_id"`
		VariantID *int64 `db:"product_variant_id"`
		Position  int    `db:"position"`
	}
	if err = tx.SelectContext(ctx, &rows, `SELECT id,product_id,product_variant_id,position FROM wishlist_items WHERE wishlist_id=$1 ORDER BY position,id`, meta.ID); err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ProductID)
	}
	cards := []usecase.ProductCard{}
	positions := map[int64]int{}
	if len(ids) > 0 {
		var public []productCardRow
		if err = tx.SelectContext(ctx, &public, `SELECT `+productColumns+`,b.name AS brand_name,b.slug AS brand_slug`+publicProducts+` AND p.id=ANY($1) ORDER BY p.id`, pq.Array(ids)); err != nil {
			return nil, err
		}
		publicIDs := []int64{}
		for _, r := range public {
			p, e := r.product()
			if e != nil {
				return nil, e
			}
			positions[p.ID()] = len(cards)
			publicIDs = append(publicIDs, p.ID())
			cards = append(cards, usecase.ProductCard{Product: *p, BrandName: r.BrandName, BrandSlug: r.BrandSlug, Colors: []usecase.ProductCardColor{}})
		}
		if err = loadCardChildren(ctx, tx, cards, publicIDs, positions); err != nil {
			return nil, err
		}
	}
	detail := &usecase.WishlistDetail{Wishlist: meta.entity(), Items: []usecase.WishlistItem{}}
	for _, r := range rows {
		index, visible := positions[r.ProductID]
		// Public links never reveal a hidden product's identifiers or a tombstone.
		if hash != nil && !visible {
			continue
		}
		item := usecase.WishlistItem{ID: r.ID, ProductID: r.ProductID, VariantID: r.VariantID, Position: r.Position}
		if visible {
			item.Product = &cards[index]
			if r.VariantID != nil {
				for _, color := range cards[index].Colors {
					for _, v := range color.Variants {
						if v.ID == *r.VariantID {
							item.Variant = &usecase.WishlistVariant{ID: v.ID, Color: color.Color, Size: v.Size, Stock: v.Stock}
						}
					}
				}
			}
		}
		// Public positions are compact: gaps must not reveal the hidden count.
		if hash != nil {
			item.Position = len(detail.Items)
		}
		detail.Items = append(detail.Items, item)
	}
	detail.Wishlist.ItemCount = len(detail.Items)
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return detail, nil
}
func (s *Wishlists) Create(ctx context.Context, uid int64, title, description string) (int64, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	// Serialize list creation for this owner, including when none exists yet.
	var owner int64
	if err = tx.GetContext(ctx, &owner, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, uid); err != nil {
		return 0, err
	}
	var count int
	if err = tx.GetContext(ctx, &count, `SELECT count(*) FROM wishlists WHERE owner_user_id=$1`, uid); err != nil {
		return 0, err
	}
	if count >= entities.MaxWishlists {
		return 0, usecase.ErrWishlistLimit
	}
	var id int64
	if err = tx.GetContext(ctx, &id, `INSERT INTO wishlists(owner_user_id,title,description) VALUES($1,$2,$3) RETURNING id`, uid, title, description); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}
func (s *Wishlists) within(ctx context.Context, uid, id int64, fn func(*sqlx.Tx, *wishlistRow) error) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var row wishlistRow
	err = tx.GetContext(ctx, &row, `SELECT `+wishlistColumns+` FROM wishlists WHERE id=$1 AND owner_user_id=$2 FOR UPDATE`, id, uid)
	if errors.Is(err, sql.ErrNoRows) {
		return usecase.ErrResourceHidden
	}
	if err != nil {
		return err
	}
	if err = fn(tx, &row); err != nil {
		return err
	}
	return tx.Commit()
}
func touchWishlist(ctx context.Context, tx *sqlx.Tx, id int64) error {
	_, err := tx.ExecContext(ctx, `UPDATE wishlists SET updated_at=clock_timestamp() WHERE id=$1`, id)
	return err
}
func (s *Wishlists) Update(ctx context.Context, uid, id int64, title, description *string) error {
	return s.within(ctx, uid, id, func(tx *sqlx.Tx, row *wishlistRow) error {
		if title != nil {
			row.Title = *title
		}
		if description != nil {
			row.Description = *description
		}
		t, d, err := entities.WishlistText(row.Title, row.Description)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE wishlists SET title=$2,description=$3,updated_at=clock_timestamp() WHERE id=$1`, id, t, d)
		return err
	})
}
func (s *Wishlists) Delete(ctx context.Context, uid, id int64) error {
	return s.within(ctx, uid, id, func(tx *sqlx.Tx, _ *wishlistRow) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM wishlists WHERE id=$1`, id)
		return err
	})
}

// Catalog parent locks precede item writes, in the same order as management.
func lockWishlistProduct(ctx context.Context, tx *sqlx.Tx, product int64, variant *int64) error {
	var brand, id int64
	err := tx.GetContext(ctx, &brand, `SELECT brand_id FROM products WHERE id=$1`, product)
	if errors.Is(err, sql.ErrNoRows) {
		return usecase.ErrResourceHidden
	}
	if err != nil {
		return err
	}
	if err = tx.GetContext(ctx, &id, `SELECT id FROM brands WHERE id=$1 FOR SHARE`, brand); err != nil {
		return err
	}
	err = tx.GetContext(ctx, &id, `SELECT p.id`+publicProducts+` AND p.id=$1 AND p.brand_id=$2 FOR SHARE OF p`, product, brand)
	if errors.Is(err, sql.ErrNoRows) {
		return usecase.ErrResourceHidden
	}
	if err != nil {
		return err
	}
	if variant != nil {
		err = tx.GetContext(ctx, &id, `SELECT v.id FROM product_variants v JOIN product_colors c ON c.id=v.product_color_id WHERE v.id=$1 AND c.product_id=$2`, *variant, product)
		if errors.Is(err, sql.ErrNoRows) {
			return usecase.ErrResourceHidden
		}
	}
	return err
}
func (s *Wishlists) AddItem(ctx context.Context, uid, id, product int64, variant *int64) error {
	return s.within(ctx, uid, id, func(tx *sqlx.Tx, _ *wishlistRow) error {
		if err := lockWishlistProduct(ctx, tx, product, variant); err != nil {
			return err
		}
		var count int
		if err := tx.GetContext(ctx, &count, `SELECT count(*) FROM wishlist_items WHERE wishlist_id=$1`, id); err != nil {
			return err
		}
		if count >= entities.MaxWishlistItems {
			return usecase.ErrWishlistLimit
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO wishlist_items(wishlist_id,product_id,product_variant_id,position) VALUES($1,$2,$3,$4)`, id, product, variant, count)
		var pe *pq.Error
		if errors.As(err, &pe) && pe.Code == "23505" {
			return usecase.ErrWishlistDuplicate
		}
		if err != nil {
			return err
		}
		return touchWishlist(ctx, tx, id)
	})
}
func (s *Wishlists) ChangeItem(ctx context.Context, uid, id, item int64, variant *int64) error {
	return s.within(ctx, uid, id, func(tx *sqlx.Tx, _ *wishlistRow) error {
		var product int64
		err := tx.GetContext(ctx, &product, `SELECT product_id FROM wishlist_items WHERE id=$1 AND wishlist_id=$2`, item, id)
		if errors.Is(err, sql.ErrNoRows) {
			return usecase.ErrResourceHidden
		}
		if err != nil {
			return err
		}
		if variant != nil {
			if err = lockWishlistProduct(ctx, tx, product, variant); err != nil {
				return err
			}
		}
		if _, err = tx.ExecContext(ctx, `UPDATE wishlist_items SET product_variant_id=$3 WHERE id=$1 AND wishlist_id=$2`, item, id, variant); err != nil {
			return err
		}
		return touchWishlist(ctx, tx, id)
	})
}
func (s *Wishlists) RemoveItem(ctx context.Context, uid, id, item int64) error {
	return s.within(ctx, uid, id, func(tx *sqlx.Tx, _ *wishlistRow) error {
		if err := changedRow(tx.ExecContext(ctx, `DELETE FROM wishlist_items WHERE id=$1 AND wishlist_id=$2`, item, id)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `SET CONSTRAINTS wishlist_item_position DEFERRED`); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `WITH ranked AS (SELECT id,(row_number() OVER(ORDER BY position,id)-1)::integer AS pos FROM wishlist_items WHERE wishlist_id=$1)
 UPDATE wishlist_items i SET position=r.pos FROM ranked r WHERE i.id=r.id`, id)
		if err != nil {
			return err
		}
		return touchWishlist(ctx, tx, id)
	})
}
func (s *Wishlists) Reorder(ctx context.Context, uid, id int64, items []int64) error {
	return s.within(ctx, uid, id, func(tx *sqlx.Tx, _ *wishlistRow) error {
		var current []int64
		if err := tx.SelectContext(ctx, &current, `SELECT id FROM wishlist_items WHERE wishlist_id=$1 ORDER BY id`, id); err != nil {
			return err
		}
		if len(current) != len(items) {
			return usecase.ErrWishlistChanged
		}
		known := map[int64]bool{}
		for _, v := range current {
			known[v] = true
		}
		for _, v := range items {
			if !known[v] {
				return usecase.ErrWishlistChanged
			}
		}
		if _, err := tx.ExecContext(ctx, `SET CONSTRAINTS wishlist_item_position DEFERRED`); err != nil {
			return err
		}
		for position, item := range items {
			if _, err := tx.ExecContext(ctx, `UPDATE wishlist_items SET position=$3 WHERE id=$1 AND wishlist_id=$2`, item, id, position); err != nil {
				return err
			}
		}
		return touchWishlist(ctx, tx, id)
	})
}
func (s *Wishlists) Share(ctx context.Context, uid, id int64, hash, sealed []byte) error {
	return s.within(ctx, uid, id, func(tx *sqlx.Tx, _ *wishlistRow) error {
		visibility := "private"
		var digest, ciphertext any
		if hash != nil {
			visibility = "public"
			digest, ciphertext = hash, sealed
		}
		// Untyped nil is SQL NULL; a typed []byte(nil) is encoded by lib/pq as
		// an empty bytea and would violate the private-list constraint.
		_, err := tx.ExecContext(ctx, `UPDATE wishlists SET visibility=$2,share_token_hash=$3,share_token_sealed=$4,updated_at=clock_timestamp() WHERE id=$1`, id, visibility, digest, ciphertext)
		return err
	})
}
