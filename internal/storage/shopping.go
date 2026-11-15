package storage

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
	"srez/internal/usecase"
)

type Shopping struct{ db *sqlx.DB }

func NewShopping(db *sqlx.DB) *Shopping { return &Shopping{db} }

var _ usecase.ShoppingRepository = (*Shopping)(nil)

func (s *Shopping) Favorites(ctx context.Context, uid int64) ([]usecase.Favorite, error) {
	tx, err := s.db.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var ids []int64
	if err = tx.SelectContext(ctx, &ids, `SELECT product_id FROM favorites WHERE user_id=$1 ORDER BY product_id`, uid); err != nil {
		return nil, err
	}
	var rows []productCardRow
	if err = tx.SelectContext(ctx, &rows, `SELECT `+productColumns+`,b.name AS brand_name,b.slug AS brand_slug`+publicProducts+`
 AND EXISTS(SELECT 1 FROM favorites f WHERE f.user_id=$1 AND f.product_id=p.id) ORDER BY p.id`, uid); err != nil {
		return nil, err
	}
	cards := make([]usecase.ProductCard, 0, len(rows))
	publicIDs := []int64{}
	positions := map[int64]int{}
	for _, row := range rows {
		p, err := row.product()
		if err != nil {
			return nil, err
		}
		positions[p.ID()] = len(cards)
		publicIDs = append(publicIDs, p.ID())
		cards = append(cards, usecase.ProductCard{Product: *p, BrandName: row.BrandName, BrandSlug: row.BrandSlug, Colors: []usecase.ProductCardColor{}})
	}
	if err = loadCardChildren(ctx, tx, cards, publicIDs, positions); err != nil {
		return nil, err
	}
	result := make([]usecase.Favorite, 0, len(ids))
	for _, id := range ids {
		favorite := usecase.Favorite{ProductID: id}
		if i, ok := positions[id]; ok {
			favorite.Product = &cards[i]
		}
		result = append(result, favorite)
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}
func (s *Shopping) AddFavorite(ctx context.Context, uid, productID int64) error {
	// Serialize the publication/status check with management. No stock check:
	// published products with stock=0 can still be saved.
	return s.WithinCart(ctx, uid, func(adapter usecase.CartTx) error {
		t := adapter.(*cartTx)
		var brand int64
		err := t.tx.GetContext(ctx, &brand, `SELECT brand_id FROM products WHERE id=$1`, productID)
		if errors.Is(err, sql.ErrNoRows) {
			return usecase.ErrResourceHidden
		}
		if err != nil {
			return err
		}
		var id int64
		if err = t.tx.GetContext(ctx, &id, `SELECT id FROM brands WHERE id=$1 FOR UPDATE`, brand); err != nil {
			return err
		}
		err = t.tx.GetContext(ctx, &id, `SELECT p.id FROM products p JOIN brands b ON b.id=p.brand_id
 WHERE p.id=$1 AND p.brand_id=$2 AND p.status='published' AND b.status='active' FOR UPDATE OF p`, productID, brand)
		if errors.Is(err, sql.ErrNoRows) {
			return usecase.ErrResourceHidden
		}
		if err != nil {
			return err
		}
		_, err = t.tx.ExecContext(ctx, `INSERT INTO favorites(user_id,product_id) VALUES($1,$2) ON CONFLICT(user_id,product_id) DO NOTHING`, uid, productID)
		return err
	})
}
func (s *Shopping) RemoveFavorite(ctx context.Context, uid, productID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM favorites WHERE user_id=$1 AND product_id=$2`, uid, productID)
	return err
}

// A transaction advisory lock serializes mutations and checkout for one buyer,
// including an empty cart. Management never takes this lock. Catalog locks must
// be acquired before cart row locks, since management can detach deleted variants.
func (s *Shopping) WithinCart(ctx context.Context, uid int64, fn func(usecase.CartTx) error) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Negative user IDs form a namespace disjoint from existing positive migration
	// and seed advisory lock keys. User IDs are positive BIGINTs.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(-$1::bigint)`, uid); err != nil {
		return err
	}
	if err = fn(&cartTx{tx: tx, userID: uid}); err != nil {
		return err
	}
	return tx.Commit()
}

type cartTx struct {
	tx     *sqlx.Tx
	userID int64
}

var _ usecase.CartTx = (*cartTx)(nil)

func (t *cartTx) Item(ctx context.Context, id int64) (*usecase.CartLine, error) {
	var r struct {
		ID        int64  `db:"id"`
		VariantID *int64 `db:"product_variant_id"`
		Quantity  int64  `db:"quantity"`
	}
	err := t.tx.GetContext(ctx, &r, `SELECT id,product_variant_id,quantity FROM cart_items WHERE id=$1 AND user_id=$2`, id, t.userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &usecase.CartLine{ID: r.ID, VariantID: r.VariantID, Quantity: r.Quantity}, nil
}
func (t *cartTx) LockVariant(ctx context.Context, variantID int64) (*usecase.CartProduct, error) {
	var ref struct {
		BrandID   int64 `db:"brand_id"`
		ProductID int64 `db:"product_id"`
	}
	err := t.tx.GetContext(ctx, &ref, `SELECT p.brand_id,p.id AS product_id FROM product_variants v
 JOIN product_colors c ON c.id=v.product_color_id JOIN products p ON p.id=c.product_id WHERE v.id=$1`, variantID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var id int64
	if err = t.tx.GetContext(ctx, &id, `SELECT id FROM brands WHERE id=$1 FOR UPDATE`, ref.BrandID); err != nil {
		return nil, err
	}
	if err = t.tx.GetContext(ctx, &id, `SELECT id FROM products WHERE id=$1 AND brand_id=$2 FOR UPDATE`, ref.ProductID, ref.BrandID); err != nil {
		return nil, err
	}
	var row cartProductRow
	err = t.tx.GetContext(ctx, &row, `SELECT p.id AS product_id,p.brand_id,v.id AS variant_id,p.name,c.color,v.size,
 p.price_kopecks AS unit_price_kopecks,v.stock,COALESCE((SELECT url FROM product_color_images WHERE product_color_id=c.id ORDER BY position LIMIT 1),'') AS preview_image_url
 FROM product_variants v JOIN product_colors c ON c.id=v.product_color_id JOIN products p ON p.id=c.product_id JOIN brands b ON b.id=p.brand_id
 WHERE v.id=$1 AND p.id=$2 AND p.brand_id=$3 AND p.status='published' AND b.status='active' FOR UPDATE OF c,v`, variantID, ref.ProductID, ref.BrandID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return row.product(), nil
}
func (t *cartTx) VariantQuantity(ctx context.Context, variant int64) (int64, error) {
	var quantity int64
	err := t.tx.GetContext(ctx, &quantity, `SELECT quantity FROM cart_items WHERE user_id=$1 AND product_variant_id=$2`, t.userID, variant)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return quantity, err
}
func (t *cartTx) Add(ctx context.Context, variant, quantity int64) error {
	_, err := t.tx.ExecContext(ctx, `INSERT INTO cart_items(user_id,product_variant_id,quantity) VALUES($1,$2,$3)
 ON CONFLICT(user_id,product_variant_id) DO UPDATE SET quantity=cart_items.quantity+excluded.quantity`, t.userID, variant, quantity)
	return err
}
func (t *cartTx) Set(ctx context.Context, id, quantity int64) error {
	return changedRow(t.tx.ExecContext(ctx, `UPDATE cart_items SET quantity=$3 WHERE id=$1 AND user_id=$2`, id, t.userID, quantity))
}
func (t *cartTx) Remove(ctx context.Context, id int64) error {
	return changedRow(t.tx.ExecContext(ctx, `DELETE FROM cart_items WHERE id=$1 AND user_id=$2`, id, t.userID))
}

type cartProductRow struct {
	ProductID        int64  `db:"product_id"`
	BrandID          int64  `db:"brand_id"`
	VariantID        int64  `db:"variant_id"`
	Name             string `db:"name"`
	Color            string `db:"color"`
	Size             string `db:"size"`
	PreviewImageURL  string `db:"preview_image_url"`
	UnitPriceKopecks int64  `db:"unit_price_kopecks"`
	Stock            int64  `db:"stock"`
}

func (r cartProductRow) product() *usecase.CartProduct {
	return &usecase.CartProduct{ProductID: r.ProductID, BrandID: r.BrandID, VariantID: r.VariantID, Name: r.Name, Color: r.Color, Size: r.Size, PreviewImageURL: r.PreviewImageURL, UnitPriceKopecks: r.UnitPriceKopecks, Stock: r.Stock}
}
func cartLines(ctx context.Context, db sqlx.QueryerContext, uid int64) ([]usecase.CartLine, error) {
	var rows []struct {
		ID       int64  `db:"id"`
		Variant  *int64 `db:"product_variant_id"`
		Quantity int64  `db:"quantity"`
		cartProductRow
	}
	// Private catalog values are redacted in SQL, not just in the HTTP projection.
	err := sqlx.SelectContext(ctx, db, &rows, `SELECT ci.id,ci.product_variant_id,ci.quantity,
 CASE WHEN b.id IS NOT NULL THEN p.id ELSE 0 END AS product_id,
 COALESCE(b.id,0) AS brand_id,COALESCE(v.id,0) AS variant_id,
 CASE WHEN b.id IS NOT NULL THEN p.name ELSE '' END AS name,
 CASE WHEN b.id IS NOT NULL THEN c.color ELSE '' END AS color,
 CASE WHEN b.id IS NOT NULL THEN v.size ELSE '' END AS size,
 CASE WHEN b.id IS NOT NULL THEN p.price_kopecks ELSE 0 END AS unit_price_kopecks,
 CASE WHEN b.id IS NOT NULL THEN v.stock ELSE 0 END AS stock,
 CASE WHEN b.id IS NOT NULL THEN COALESCE((SELECT url FROM product_color_images WHERE product_color_id=c.id ORDER BY position LIMIT 1),'') ELSE '' END AS preview_image_url
 FROM cart_items ci LEFT JOIN product_variants v ON v.id=ci.product_variant_id
 LEFT JOIN product_colors c ON c.id=v.product_color_id
 LEFT JOIN products p ON p.id=c.product_id AND p.status='published'
 LEFT JOIN brands b ON b.id=p.brand_id AND b.status='active'
 WHERE ci.user_id=$1 ORDER BY ci.id`, uid)
	if err != nil {
		return nil, err
	}
	lines := make([]usecase.CartLine, 0, len(rows))
	for _, row := range rows {
		line := usecase.CartLine{ID: row.ID, VariantID: row.Variant, Quantity: row.Quantity}
		if row.ProductID > 0 {
			line.Product = row.product()
		}
		lines = append(lines, line)
	}
	return lines, nil
}
func (s *Shopping) Cart(ctx context.Context, uid int64) ([]usecase.CartLine, error) {
	return cartLines(ctx, s.db, uid)
}
func (t *cartTx) Lines(ctx context.Context) ([]usecase.CartLine, error) {
	return cartLines(ctx, t.tx, t.userID)
}
