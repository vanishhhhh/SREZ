package storage_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"srez/internal/entities"
	"srez/internal/storage"
	"srez/internal/usecase"
)

func reservationOrder(t *testing.T, f pvmFixture) *entities.Order {
	t.Helper()
	ctx := context.Background()
	_, err := usecase.NewShopping(storage.NewShopping(f.db)).ChangeCart(ctx, f.owner, f.variant, 1, true)
	pvmCheck(t, err)
	o, err := usecase.NewOrders(storage.NewOrders(f.db)).Checkout(ctx, f.owner)
	pvmCheck(t, err)
	return o
}
func reservationCounts(t *testing.T, db *sqlx.DB, id int64, active, released int) {
	t.Helper()
	var a, r int
	pvmCheck(t, db.Get(&a, `SELECT count(*) FROM order_reservations WHERE order_id=$1 AND released_at IS NULL`, id))
	pvmCheck(t, db.Get(&r, `SELECT count(*) FROM order_reservations WHERE order_id=$1 AND released_at IS NOT NULL`, id))
	if a != active || r != released {
		t.Fatalf("reservations: active=%d released=%d", a, r)
	}
}
func TestPostgresReservationsLastUnitAvailableAfterCancel(t *testing.T) {
	f := newPVMFixture(t, true)
	ctx := context.Background()
	o := reservationOrder(t, f)
	auditStock(t, f, 0)
	reservationCounts(t, f.db, o.ID(), 1, 0)
	other := managedUser(t, usecase.NewUsers(storage.NewUsers(f.db)), "reserve_other")
	shop := usecase.NewShopping(storage.NewShopping(f.db))
	orders := usecase.NewOrders(storage.NewOrders(f.db))
	if _, err := shop.ChangeCart(ctx, other, f.variant, 1, true); !errors.Is(err, usecase.ErrInsufficientStock) {
		t.Fatal("held unit was available to another buyer", err)
	}
	_, err := orders.Cancel(ctx, f.owner, o.ID())
	pvmCheck(t, err)
	auditStock(t, f, 1)
	reservationCounts(t, f.db, o.ID(), 0, 1)
	_, err = shop.ChangeCart(ctx, other, f.variant, 1, true)
	pvmCheck(t, err)
	second, err := orders.Checkout(ctx, other)
	pvmCheck(t, err)
	auditStock(t, f, 0)
	reservationCounts(t, f.db, second.ID(), 1, 0)
	_, err = orders.Cancel(ctx, f.owner, o.ID())
	pvmCheck(t, err)
	auditStock(t, f, 0)
}
func TestPostgresReservationsConcurrentExpiryAndCancellation(t *testing.T) {
	for _, operation := range []string{"expiry", "cancel"} {
		t.Run(operation, func(t *testing.T) {
			f := newPVMFixture(t, true)
			o := reservationOrder(t, f)
			a, _ := auditConnection(t, f.db)
			b, _ := auditConnection(t, f.db)
			start := make(chan struct{})
			results := make(chan error, 2)
			go func() {
				<-start
				_, err := usecase.NewOrders(storage.NewOrders(a)).Expire(context.Background(), o.CancelBefore(), 100)
				results <- err
			}()
			go func() {
				<-start
				orders := usecase.NewOrders(storage.NewOrders(b))
				var err error
				if operation == "expiry" {
					_, err = orders.Expire(context.Background(), o.CancelBefore(), 100)
				} else {
					_, err = orders.Cancel(context.Background(), f.owner, o.ID())
				}
				results <- err
			}()
			close(start)
			for range 2 {
				err := <-results
				if err != nil && !errors.Is(err, entities.ErrOrderCancellationClosed) {
					t.Fatal(err)
				}
			}
			auditStock(t, f, 1)
			reservationCounts(t, f.db, o.ID(), 0, 1)
			stored, err := usecase.NewOrders(storage.NewOrders(f.db)).Get(context.Background(), f.owner, o.ID())
			pvmCheck(t, err)
			if stored.Status() != "cancelled" && stored.Status() != "expired" {
				t.Fatal(stored.Status())
			}
		})
	}
}
func TestPostgresReservationsWorkerRunsWithoutBuyerRequests(t *testing.T) {
	f := newPVMFixture(t, true)
	o := reservationOrder(t, f)
	auditStock(t, f, 0)
	exec(t, f.db, `UPDATE orders SET created_at=now()-interval '16 minutes',cancel_before=now()-interval '1 minute' WHERE id=$1`, o.ID())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	errs := make(chan error, 1)
	orders := usecase.NewOrders(storage.NewOrders(f.db))
	go func() {
		defer close(done)
		orders.RunReservationExpiry(ctx, 10*time.Millisecond, func(err error) {
			select {
			case errs <- err:
			default:
			}
		})
	}()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("expiry worker did not stop")
		}
	}()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		var status string
		pvmCheck(t, f.db.Get(&status, `SELECT status FROM orders WHERE id=$1`, o.ID()))
		if status == "expired" {
			break
		}
		select {
		case err := <-errs:
			t.Fatal(err)
		case <-deadline.C:
			t.Fatal("expired hold required a buyer request")
		case <-tick.C:
		}
	}
	auditStock(t, f, 1)
	reservationCounts(t, f.db, o.ID(), 0, 1)
	if _, err := orders.Cancel(context.Background(), f.owner, o.ID()); !errors.Is(err, entities.ErrOrderCancellationClosed) {
		t.Fatal("expired order became cancellable", err)
	}
}
func TestPostgresReservationsReleaseFailureRollsBackEveryLine(t *testing.T) {
	f := newPVMFixture(t, true)
	auditSetStock(t, f, 10)
	ctx := context.Background()
	second, err := f.m.CreateVariant(ctx, f.owner, f.product, f.color, "M", 10, "")
	pvmCheck(t, err)
	shop := usecase.NewShopping(storage.NewShopping(f.db))
	orders := usecase.NewOrders(storage.NewOrders(f.db))
	for _, v := range []int64{f.variant, second.ID()} {
		_, err = shop.ChangeCart(ctx, f.owner, v, 1, true)
		pvmCheck(t, err)
	}
	o, err := orders.Checkout(ctx, f.owner)
	pvmCheck(t, err)
	auditStock(t, f, 9)
	exec(t, f.db, `UPDATE product_variants SET stock=9223372036854775807 WHERE id=$1`, second.ID())
	if _, err = orders.Cancel(ctx, f.owner, o.ID()); !errors.Is(err, usecase.ErrInventoryConflict) {
		t.Fatal(err)
	}
	auditStock(t, f, 9)
	reservationCounts(t, f.db, o.ID(), 2, 0)
	stored, err := orders.Get(ctx, f.owner, o.ID())
	pvmCheck(t, err)
	if stored.Status() != "unpaid" {
		t.Fatal("failed release changed order")
	}
	exec(t, f.db, `UPDATE product_variants SET stock=9 WHERE id=$1`, second.ID())
	_, err = orders.Cancel(ctx, f.owner, o.ID())
	pvmCheck(t, err)
	auditStock(t, f, 10)
	reservationCounts(t, f.db, o.ID(), 0, 2)
}
