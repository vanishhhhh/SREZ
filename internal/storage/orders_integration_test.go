package storage_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"srez/internal/devseed"
	"srez/internal/storage"
	"srez/internal/usecase"
)

func TestPostgresOrdersMultiBrandSnapshotsAndOwnership(t *testing.T) {
	f := newPVMFixture(t, true)
	ctx := context.Background()
	shop := usecase.NewShopping(storage.NewShopping(f.db))
	orders := usecase.NewOrders(storage.NewOrders(f.db))
	pvmCheck(t, devseed.Apply(ctx, f.db))
	var second int64
	pvmCheck(t, f.db.Get(&second, `SELECT v.id FROM product_variants v JOIN product_colors c ON c.id=v.product_color_id JOIN products p ON p.id=c.product_id WHERE p.brand_id<>$1 AND v.stock>0 ORDER BY v.id LIMIT 1`, f.brand))
	_, err := shop.ChangeCart(ctx, f.owner, f.variant, 1, true)
	pvmCheck(t, err)
	before, err := shop.ChangeCart(ctx, f.owner, second, 2, true)
	pvmCheck(t, err)
	order, err := orders.Checkout(ctx, f.owner)
	pvmCheck(t, err)
	if order.Status() != "unpaid" || order.TotalKopecks() != before.TotalKopecks || len(order.Lines()) != 2 || order.Lines()[0].BrandID == order.Lines()[1].BrandID {
		t.Fatal(order.State())
	}
	if count(t, f.db, "orders") != 1 || count(t, f.db, "order_items") != 2 || count(t, f.db, "cart_items") != 0 {
		t.Fatal("not one multi-brand order")
	}
	for _, line := range before.Items {
		var stock int64
		pvmCheck(t, f.db.Get(&stock, `SELECT stock FROM product_variants WHERE id=$1`, *line.VariantID))
		if stock != line.Product.Stock-line.Quantity {
			t.Fatal("reservation did not reduce available stock", stock)
		}
	}
	stored, err := orders.Get(ctx, f.owner, order.ID())
	pvmCheck(t, err)
	if !reflect.DeepEqual(stored.State(), order.State()) {
		t.Fatal("response differs from stored order")
	}
	other := managedUser(t, usecase.NewUsers(storage.NewUsers(f.db)), "order_outsider")
	if _, err = orders.Get(ctx, other, order.ID()); !errors.Is(err, usecase.ErrResourceHidden) {
		t.Fatal("foreign order exposed", err)
	}
	if raw, err := storage.NewOrders(f.db).FindOrder(ctx, other, order.ID()); err != nil || raw != nil {
		t.Fatal("storage ownership missing", raw, err)
	}
	list, err := orders.List(ctx, other)
	pvmCheck(t, err)
	if len(list) != 0 {
		t.Fatal("foreign list exposed")
	}
	name := "Changed product"
	price := int64(50000)
	_, err = f.m.ChangeProduct(ctx, f.owner, f.product, usecase.ProductPatch{Name: &name, PriceKopecks: &price}, "patch")
	pvmCheck(t, err)
	_, err = f.m.RenameColor(ctx, f.owner, f.product, f.color, "Red")
	pvmCheck(t, err)
	size := "XL"
	_, err = f.m.ChangeVariant(ctx, f.owner, f.product, f.color, f.variant, usecase.VariantPatch{Size: &size})
	pvmCheck(t, err)
	after, err := orders.Get(ctx, f.owner, order.ID())
	pvmCheck(t, err)
	if !reflect.DeepEqual(stored.State(), after.State()) {
		t.Fatal("snapshot changed with price/name/color/size")
	}
	_, err = f.m.ChangeProduct(ctx, f.owner, f.product, usecase.ProductPatch{}, "hide")
	pvmCheck(t, err)
	pvmCheck(t, f.m.DeleteColor(ctx, f.owner, f.product, f.color))
	after, err = orders.Get(ctx, f.owner, order.ID())
	pvmCheck(t, err)
	if !reflect.DeepEqual(stored.State(), after.State()) {
		t.Fatal("history depends on live variant")
	}
	if _, err = orders.Checkout(ctx, f.owner); !errors.Is(err, usecase.ErrEmptyCart) {
		t.Fatal("repeat checkout", err)
	}
	if count(t, f.db, "orders") != 1 {
		t.Fatal("repeat created order")
	}
	list, err = orders.List(ctx, f.owner)
	pvmCheck(t, err)
	if len(list) != 1 || !reflect.DeepEqual(stored.State(), list[0].State()) {
		t.Fatal("history list", list)
	}
	cancelled, err := orders.Cancel(ctx, f.owner, order.ID())
	pvmCheck(t, err)
	if cancelled.Status() != "cancelled" || !reflect.DeepEqual(cancelled.Lines(), stored.Lines()) {
		t.Fatal("deleted catalog prevented snapshot-preserving cancellation")
	}
}
func TestPostgresOrdersConcurrentLastUnitReservation(t *testing.T) {
	f := newPVMFixture(t, true)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	shop := usecase.NewShopping(storage.NewShopping(f.db))
	orders := usecase.NewOrders(storage.NewOrders(f.db))
	other := managedUser(t, usecase.NewUsers(storage.NewUsers(f.db)), "last_unit_buyer")
	for _, uid := range []int64{f.owner, other} {
		_, err := shop.ChangeCart(ctx, uid, f.variant, 1, true)
		pvmCheck(t, err)
	}
	type outcome struct {
		uid int64
		err error
	}
	start := make(chan struct{})
	results := make(chan outcome, 2)
	for _, uid := range []int64{f.owner, other} {
		go func() { <-start; _, err := orders.Checkout(ctx, uid); results <- outcome{uid, err} }()
	}
	close(start)
	success := 0
	for range 2 {
		r := <-results
		cart, err := shop.Cart(ctx, r.uid)
		pvmCheck(t, err)
		if r.err == nil {
			success++
			if len(cart.Items) != 0 {
				t.Fatal("quote cart not cleared")
			}
		} else {
			if !errors.Is(r.err, usecase.ErrInsufficientStock) {
				t.Fatal(r.err)
			}
			if len(cart.Items) != 1 || cart.Items[0].Quantity != 1 {
				t.Fatal("loser cart changed")
			}
		}
	}
	var stock int64
	pvmCheck(t, f.db.Get(&stock, `SELECT stock FROM product_variants WHERE id=$1`, f.variant))
	if success != 1 || stock != 0 || count(t, f.db, "orders") != 1 || count(t, f.db, "order_items") != 1 || count(t, f.db, "cart_items") != 1 {
		t.Fatal("last unit race", success, stock)
	}
}
func TestPostgresOrdersConcurrentSameCart(t *testing.T) {
	f := newPVMFixture(t, true)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	shop := usecase.NewShopping(storage.NewShopping(f.db))
	orders := usecase.NewOrders(storage.NewOrders(f.db))
	stock := int64(5)
	_, err := f.m.ChangeVariant(ctx, f.owner, f.product, f.color, f.variant, usecase.VariantPatch{Stock: &stock})
	pvmCheck(t, err)
	_, err = shop.ChangeCart(ctx, f.owner, f.variant, 2, true)
	pvmCheck(t, err)
	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() { <-start; _, err := orders.Checkout(ctx, f.owner); results <- err }()
	}
	close(start)
	success, empty := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			success++
		} else if errors.Is(err, usecase.ErrEmptyCart) {
			empty++
		} else {
			t.Fatal(err)
		}
	}
	pvmCheck(t, f.db.Get(&stock, `SELECT stock FROM product_variants WHERE id=$1`, f.variant))
	if success != 1 || empty != 1 || stock != 3 || count(t, f.db, "orders") != 1 || count(t, f.db, "cart_items") != 0 {
		t.Fatal(success, empty, stock)
	}
}
func TestPostgresOrdersRollbackLeavesWholeCart(t *testing.T) {
	for _, failure := range []string{"stock", "hidden", "deleted", "write"} {
		t.Run(failure, func(t *testing.T) {
			f := newPVMFixture(t, true)
			ctx := context.Background()
			shop := usecase.NewShopping(storage.NewShopping(f.db))
			orders := usecase.NewOrders(storage.NewOrders(f.db))
			second, err := f.m.CreateVariant(ctx, f.owner, f.product, f.color, "M", 1, "")
			pvmCheck(t, err)
			_, err = shop.ChangeCart(ctx, f.owner, f.variant, 1, true)
			pvmCheck(t, err)
			_, err = shop.ChangeCart(ctx, f.owner, second.ID(), 1, true)
			pvmCheck(t, err)
			var want error
			switch failure {
			case "stock":
				stock := int64(0)
				_, err = f.m.ChangeVariant(ctx, f.owner, f.product, f.color, second.ID(), usecase.VariantPatch{Stock: &stock})
				pvmCheck(t, err)
				want = usecase.ErrInsufficientStock
			case "hidden":
				_, err = f.m.ChangeProduct(ctx, f.owner, f.product, usecase.ProductPatch{}, "hide")
				pvmCheck(t, err)
				want = usecase.ErrCartUnavailable
			case "deleted":
				pvmCheck(t, f.m.DeleteVariant(ctx, f.owner, f.product, f.color, second.ID()))
				want = usecase.ErrCartUnavailable
			case "write":
				// Fail after the first line was inserted and its cart row removed. The
				// surrounding transaction must restore every row, including the first line.
				exec(t, f.db, fmt.Sprintf(`CREATE FUNCTION fail_second_order_line() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.product_variant_id=%d THEN RAISE EXCEPTION 'injected test failure'; END IF; RETURN NEW; END $$`, second.ID()))
				exec(t, f.db, `CREATE TRIGGER fail_order_line BEFORE INSERT ON order_items FOR EACH ROW EXECUTE FUNCTION fail_second_order_line()`)
			}
			before, err := shop.Cart(ctx, f.owner)
			pvmCheck(t, err)
			var beforeStock, afterStock []struct {
				ID    int64 `db:"id"`
				Stock int64 `db:"stock"`
			}
			pvmCheck(t, f.db.Select(&beforeStock, `SELECT id,stock FROM product_variants ORDER BY id`))
			if _, err = orders.Checkout(ctx, f.owner); err == nil || (want != nil && !errors.Is(err, want)) {
				t.Fatal("checkout did not fail correctly", err)
			}
			after, err := shop.Cart(ctx, f.owner)
			pvmCheck(t, err)
			pvmCheck(t, f.db.Select(&afterStock, `SELECT id,stock FROM product_variants ORDER BY id`))
			if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(beforeStock, afterStock) || count(t, f.db, "orders") != 0 || count(t, f.db, "order_items") != 0 {
				t.Fatal("partial checkout committed")
			}
		})
	}
}

func TestPostgresOrdersConcurrentMultiBrandLockOrder(t *testing.T) {
	f := newPVMFixture(t, true)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	shop := usecase.NewShopping(storage.NewShopping(f.db))
	orders := usecase.NewOrders(storage.NewOrders(f.db))
	pvmCheck(t, devseed.Apply(ctx, f.db))
	var second int64
	pvmCheck(t, f.db.Get(&second, `SELECT v.id FROM product_variants v JOIN product_colors c ON c.id=v.product_color_id JOIN products p ON p.id=c.product_id WHERE p.brand_id<>$1 AND v.stock>=2 ORDER BY v.id LIMIT 1`, f.brand))
	stock := int64(2)
	_, err := f.m.ChangeVariant(ctx, f.owner, f.product, f.color, f.variant, usecase.VariantPatch{Stock: &stock})
	pvmCheck(t, err)
	other := managedUser(t, usecase.NewUsers(storage.NewUsers(f.db)), "multi_brand_buyer")
	for _, ref := range []struct{ uid, variant int64 }{{f.owner, f.variant}, {f.owner, second}, {other, second}, {other, f.variant}} {
		_, err := shop.ChangeCart(ctx, ref.uid, ref.variant, 1, true)
		pvmCheck(t, err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, uid := range []int64{f.owner, other} {
		go func() { <-start; _, err := orders.Checkout(ctx, uid); results <- err }()
	}
	close(start)
	for range 2 {
		pvmCheck(t, <-results)
	}
	if count(t, f.db, "orders") != 2 || count(t, f.db, "order_items") != 4 || count(t, f.db, "cart_items") != 0 {
		t.Fatal("multi-brand checkouts failed")
	}
	pvmCheck(t, f.db.Get(&stock, `SELECT stock FROM product_variants WHERE id=$1`, f.variant))
	if stock != 0 {
		t.Fatal(stock)
	}
}

func TestPostgresOrdersWaitingCheckoutReReadsPrice(t *testing.T) {
	f := newPVMFixture(t, true)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	shop := usecase.NewShopping(storage.NewShopping(f.db))
	orders := usecase.NewOrders(storage.NewOrders(f.db))
	_, err := shop.ChangeCart(ctx, f.owner, f.variant, 1, true)
	pvmCheck(t, err)
	guard, err := f.db.BeginTxx(ctx, nil)
	pvmCheck(t, err)
	defer guard.Rollback()
	var pid, id int
	pvmCheck(t, guard.GetContext(ctx, &pid, `SELECT pg_backend_pid()`))
	pvmCheck(t, guard.GetContext(ctx, &id, `SELECT id FROM brands WHERE id=$1 FOR UPDATE`, f.brand))
	result := make(chan error, 1)
	go func() {
		order, err := orders.Checkout(ctx, f.owner)
		if err == nil && order.TotalKopecks() != 33333 {
			err = fmt.Errorf("checkout used stale price: %d", order.TotalKopecks())
		}
		result <- err
	}()
	waiting := false
	for attempt := 0; attempt < 200; attempt++ {
		pvmCheck(t, f.db.GetContext(ctx, &waiting, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity a WHERE $1=ANY(pg_blocking_pids(a.pid)))`, pid))
		if waiting {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("checkout did not wait for catalog lock")
	}
	_, err = guard.ExecContext(ctx, `UPDATE products SET price_kopecks=33333 WHERE id=$1`, f.product)
	pvmCheck(t, err)
	pvmCheck(t, guard.Commit())
	pvmCheck(t, <-result)
}
