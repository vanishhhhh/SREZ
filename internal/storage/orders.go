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

type Orders struct{ db *sqlx.DB }

func NewOrders(db *sqlx.DB) *Orders { return &Orders{db} }

var _ usecase.OrderRepository = (*Orders)(nil)
var _ usecase.CheckoutTx = (*cartTx)(nil)

func (s *Orders) WithinCheckout(ctx context.Context, uid int64, fn func(usecase.CheckoutTx) error) error {
	return NewShopping(s.db).WithinCart(ctx, uid, func(tx usecase.CartTx) error { return fn(tx.(*cartTx)) })
}

// Lock all brands before any product/cart rows. Management locks just one brand;
// all multi-brand checkouts use ascending IDs. A preliminary read discovers only
// lock targets; the authoritative cart/catalog snapshot is read after the locks.
func (t *cartTx) LockCatalog(ctx context.Context) error {
	var brands []int64
	if err := t.tx.SelectContext(ctx, &brands, `SELECT DISTINCT p.brand_id FROM cart_items ci
 JOIN product_variants v ON v.id=ci.product_variant_id JOIN product_colors c ON c.id=v.product_color_id
 JOIN products p ON p.id=c.product_id WHERE ci.user_id=$1 ORDER BY p.brand_id`, t.userID); err != nil {
		return err
	}
	if len(brands) == 0 {
		return nil
	}
	var ids []int64
	if err := t.tx.SelectContext(ctx, &ids, `SELECT id FROM brands WHERE id=ANY($1) ORDER BY id FOR UPDATE`, pq.Array(brands)); err != nil {
		return err
	}
	if err := t.tx.SelectContext(ctx, &ids, `SELECT p.id FROM products p WHERE p.brand_id=ANY($2) AND EXISTS(
 SELECT 1 FROM cart_items ci JOIN product_variants v ON v.id=ci.product_variant_id JOIN product_colors c ON c.id=v.product_color_id
 WHERE ci.user_id=$1 AND c.product_id=p.id) ORDER BY p.id FOR UPDATE OF p`, t.userID, pq.Array(brands)); err != nil {
		return err
	}
	products := append([]int64(nil), ids...)
	if len(products) == 0 {
		return nil
	}
	if err := t.tx.SelectContext(ctx, &ids, `SELECT id FROM product_colors WHERE product_id=ANY($1) ORDER BY id FOR UPDATE`, pq.Array(products)); err != nil {
		return err
	}
	colors := append([]int64(nil), ids...)
	if err := t.tx.SelectContext(ctx, &ids, `SELECT id FROM product_color_images WHERE product_color_id=ANY($1) ORDER BY product_color_id,position FOR UPDATE`, pq.Array(colors)); err != nil {
		return err
	}
	return t.tx.SelectContext(ctx, &ids, `SELECT id FROM product_variants WHERE product_color_id=ANY($1) ORDER BY id FOR UPDATE`, pq.Array(colors))
}
func (t *cartTx) Purchase(ctx context.Context, order entities.Order) (*entities.Order, error) {
	if order.UserID() != t.userID || order.ID() != 0 || order.Status() != "unpaid" {
		return nil, entities.ErrInvalidValue
	}
	state := order.State()
	var saved orderRow
	if err := t.tx.GetContext(ctx, &saved, `INSERT INTO orders(user_id,status,total_kopecks,created_at,cancel_before) VALUES($1,'unpaid',$2,$3,$4) RETURNING id,user_id,status,total_kopecks,created_at,cancel_before,cancelled_at,expired_at`, t.userID, order.TotalKopecks(), order.CreatedAt(), order.CancelBefore()); err != nil {
		return nil, err
	}
	state.ID, state.CreatedAt, state.CancelBefore = saved.ID, saved.CreatedAt, saved.CancelBefore
	for _, line := range order.Lines() {
		// Available stock decreases as a hold, not as a paid sale.
		result, err := t.tx.ExecContext(ctx, `UPDATE product_variants v SET stock=stock-$3 WHERE v.id=$1 AND stock>=$3
 AND EXISTS(SELECT 1 FROM cart_items ci WHERE ci.user_id=$2 AND ci.product_variant_id=v.id AND ci.quantity=$3)`, line.ProductVariantID, t.userID, line.Quantity)
		if err != nil {
			return nil, err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return nil, err
		}
		if n != 1 {
			return nil, usecase.ErrInsufficientStock
		}
		amount, err := entities.LineTotal(line.UnitPriceKopecks, line.Quantity)
		if err != nil {
			return nil, err
		}
		result, err = t.tx.ExecContext(ctx, `INSERT INTO order_items(order_id,product_variant_id,product_id,brand_id,product_name,color,size,quantity,unit_price_kopecks,subtotal_kopecks)
 SELECT o.id,$3,$4,$5,$6,$7,$8,$9,$10,$11 FROM orders o JOIN cart_items ci ON ci.user_id=o.user_id AND ci.product_variant_id=$3 AND ci.quantity=$9
 WHERE o.id=$1 AND o.user_id=$2`,
			state.ID, t.userID, line.ProductVariantID, line.ProductID, line.BrandID, line.ProductName, line.Color, line.Size, line.Quantity, line.UnitPriceKopecks, amount)
		if err = changedRow(result, err); err != nil {
			return nil, err
		}
		if err = changedRow(t.tx.ExecContext(ctx, `INSERT INTO order_reservations(order_id,product_variant_id,quantity,reserved_at)
 SELECT o.id,$3,$4,o.created_at FROM orders o WHERE o.id=$1 AND o.user_id=$2`, state.ID, t.userID, line.ProductVariantID, line.Quantity)); err != nil {
			return nil, err
		}
		if err = changedRow(t.tx.ExecContext(ctx, `DELETE FROM cart_items WHERE user_id=$1 AND product_variant_id=$2 AND quantity=$3`, t.userID, line.ProductVariantID, line.Quantity)); err != nil {
			return nil, err
		}
	}
	return entities.RestoreOrder(state)
}

type orderRow struct {
	ID           int64      `db:"id"`
	UserID       int64      `db:"user_id"`
	Status       string     `db:"status"`
	TotalKopecks int64      `db:"total_kopecks"`
	CreatedAt    time.Time  `db:"created_at"`
	CancelBefore time.Time  `db:"cancel_before"`
	CancelledAt  *time.Time `db:"cancelled_at"`
	ExpiredAt    *time.Time `db:"expired_at"`
}

func (s *Orders) readOrders(ctx context.Context, uid, id int64) ([]entities.Order, error) {
	tx, err := s.db.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var rows []orderRow
	if err = tx.SelectContext(ctx, &rows, `SELECT id,user_id,status,total_kopecks,created_at,cancel_before,cancelled_at,expired_at FROM orders
 WHERE user_id=$1 AND ($2::bigint=0 OR id=$2) ORDER BY id DESC`, uid, id); err != nil {
		return nil, err
	}
	result := make([]entities.Order, 0, len(rows))
	if len(rows) == 0 {
		if err = tx.Commit(); err != nil {
			return nil, err
		}
		return result, nil
	}
	byOrder, err := readOrderLines(ctx, tx, uid, id)
	if err != nil {
		return nil, err
	}

	for _, row := range rows {
		order, err := entities.RestoreOrder(entities.OrderState{ID: row.ID, UserID: row.UserID, Status: row.Status, TotalKopecks: row.TotalKopecks, CreatedAt: row.CreatedAt, CancelBefore: row.CancelBefore, CancelledAt: row.CancelledAt, ExpiredAt: row.ExpiredAt, Lines: byOrder[row.ID]})
		if err != nil {
			return nil, err
		}
		result = append(result, *order)
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}
func (s *Orders) ListOrders(ctx context.Context, uid int64) ([]entities.Order, error) {
	return s.readOrders(ctx, uid, 0)
}
func (s *Orders) FindOrder(ctx context.Context, uid, id int64) (*entities.Order, error) {
	orders, err := s.readOrders(ctx, uid, id)
	if err != nil {
		return nil, err
	}
	if len(orders) == 0 {
		return nil, nil
	}
	return &orders[0], nil
}

func readOrderLines(ctx context.Context, tx *sqlx.Tx, uid, id int64) (map[int64][]entities.OrderLineState, error) {
	var lines []struct {
		OrderID   int64  `db:"order_id"`
		VariantID int64  `db:"product_variant_id"`
		ProductID int64  `db:"product_id"`
		BrandID   int64  `db:"brand_id"`
		Name      string `db:"product_name"`
		Color     string `db:"color"`
		Size      string `db:"size"`
		Quantity  int64  `db:"quantity"`
		Price     int64  `db:"unit_price_kopecks"`
	}
	if err := tx.SelectContext(ctx, &lines, `SELECT i.order_id,i.product_variant_id,i.product_id,i.brand_id,i.product_name,i.color,i.size,i.quantity,i.unit_price_kopecks
 FROM order_items i JOIN orders o ON o.id=i.order_id WHERE o.user_id=$1 AND ($2::bigint=0 OR o.id=$2) ORDER BY i.order_id DESC,i.id`, uid, id); err != nil {
		return nil, err
	}
	byOrder := map[int64][]entities.OrderLineState{}
	for _, line := range lines {
		byOrder[line.OrderID] = append(byOrder[line.OrderID], entities.OrderLineState{ProductVariantID: line.VariantID, ProductID: line.ProductID, BrandID: line.BrandID, ProductName: line.Name, Color: line.Color, Size: line.Size, Quantity: line.Quantity, UnitPriceKopecks: line.Price})
	}
	return byOrder, nil
}

// Reservation release follows catalog -> owned order -> reservations.
// A future payment path must coordinate with the same order/hold locks.
func (s *Orders) WithinOrder(ctx context.Context, uid, id int64, fn func(*entities.Order) error) (*entities.Order, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err = lockOrderCatalog(ctx, tx, uid, id); err != nil {
		return nil, err
	}
	var row orderRow
	err = tx.GetContext(ctx, &row, `SELECT id,user_id,status,total_kopecks,created_at,cancel_before,cancelled_at,expired_at FROM orders WHERE id=$1 AND user_id=$2 FOR UPDATE`, id, uid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, usecase.ErrResourceHidden
	}
	if err != nil {
		return nil, err
	}
	lines, err := readOrderLines(ctx, tx, uid, id)
	if err != nil {
		return nil, err
	}
	order, err := entities.RestoreOrder(entities.OrderState{ID: row.ID, UserID: row.UserID, Status: row.Status, TotalKopecks: row.TotalKopecks, CreatedAt: row.CreatedAt, CancelBefore: row.CancelBefore, CancelledAt: row.CancelledAt, ExpiredAt: row.ExpiredAt, Lines: lines[id]})
	if err != nil {
		return nil, err
	}
	if err = fn(order); err != nil {
		return nil, err
	}
	if row.Status != order.Status() {
		at := order.CancelledAt()
		if order.Status() == "expired" {
			at = order.ExpiredAt()
		}
		if at == nil {
			return nil, entities.ErrInvalidTransition
		}
		if err = releaseOrderReservations(ctx, tx, uid, id, *at); err != nil {
			return nil, err
		}
		if err = changedRow(tx.ExecContext(ctx, `UPDATE orders SET status=$3,cancelled_at=$4,expired_at=$5 WHERE id=$1 AND user_id=$2 AND status='unpaid'`, id, uid, order.Status(), order.CancelledAt(), order.ExpiredAt())); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return order, nil
}
