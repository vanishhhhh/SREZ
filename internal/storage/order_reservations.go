package storage

import (
	"context"
	"math"
	"sort"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"srez/internal/usecase"
)

func lockOrderCatalog(ctx context.Context, tx *sqlx.Tx, uid, id int64) error {
	var refs []struct {
		BrandID   int64 `db:"brand_id"`
		ProductID int64 `db:"product_id"`
	}
	if err := tx.SelectContext(ctx, &refs, `SELECT DISTINCT i.brand_id,i.product_id FROM order_items i JOIN orders o ON o.id=i.order_id WHERE o.id=$1 AND o.user_id=$2`, id, uid); err != nil {
		return err
	}
	if len(refs) == 0 {
		return usecase.ErrResourceHidden
	}
	brands, products := []int64{}, []int64{}
	for _, r := range refs {
		brands = append(brands, r.BrandID)
		products = append(products, r.ProductID)
	}
	var ids []int64
	if err := tx.SelectContext(ctx, &ids, `SELECT id FROM brands WHERE id=ANY($1) ORDER BY id FOR UPDATE`, pq.Array(brands)); err != nil {
		return err
	}
	if err := tx.SelectContext(ctx, &ids, `SELECT id FROM products WHERE id=ANY($1) ORDER BY id FOR UPDATE`, pq.Array(products)); err != nil {
		return err
	}
	if err := tx.SelectContext(ctx, &ids, `SELECT id FROM product_colors WHERE product_id=ANY($1) ORDER BY id FOR UPDATE`, pq.Array(products)); err != nil {
		return err
	}
	colors := append([]int64(nil), ids...)
	if err := tx.SelectContext(ctx, &ids, `SELECT id FROM product_color_images WHERE product_color_id=ANY($1) ORDER BY product_color_id,position FOR UPDATE`, pq.Array(colors)); err != nil {
		return err
	}
	return tx.SelectContext(ctx, &ids, `SELECT id FROM product_variants WHERE product_color_id=ANY($1) ORDER BY id FOR UPDATE`, pq.Array(colors))
}
func releaseOrderReservations(ctx context.Context, tx *sqlx.Tx, uid, id int64, at time.Time) error {
	var rows []struct {
		VariantID int64 `db:"product_variant_id"`
		Quantity  int64 `db:"quantity"`
	}
	err := tx.SelectContext(ctx, &rows, `UPDATE order_reservations r SET released_at=$3 FROM orders o
 WHERE r.order_id=o.id AND o.id=$1 AND o.user_id=$2 AND r.released_at IS NULL RETURNING r.product_variant_id,r.quantity`, id, uid, at)
	if err != nil {
		return err
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].VariantID < rows[j].VariantID })
	for _, r := range rows {
		result, err := tx.ExecContext(ctx, `UPDATE product_variants SET stock=stock+$2 WHERE id=$1 AND stock<=$3`, r.VariantID, r.Quantity, int64(math.MaxInt64)-r.Quantity)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			var exists bool
			if err = tx.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM product_variants WHERE id=$1)`, r.VariantID); err != nil {
				return err
			}
			if exists {
				return usecase.ErrInventoryConflict
			}
			// Explicit catalog deletion is respected: never recreate the variant.
		}
	}
	return nil
}

// Only the trusted expiry worker scans buyers globally. All subsequent order
// lookups and mutations still include the selected owner in SQL.
func (s *Orders) DueOrders(ctx context.Context, at time.Time, limit int) ([]usecase.DueOrder, error) {
	var rows []struct {
		ID     int64 `db:"id"`
		UserID int64 `db:"user_id"`
	}
	err := s.db.SelectContext(ctx, &rows, `SELECT id,user_id FROM orders WHERE status='unpaid' AND cancel_before<=$1 ORDER BY cancel_before,id LIMIT $2`, at, limit)
	if err != nil {
		return nil, err
	}
	result := make([]usecase.DueOrder, 0, len(rows))
	for _, r := range rows {
		result = append(result, usecase.DueOrder{ID: r.ID, UserID: r.UserID})
	}
	return result, nil
}
