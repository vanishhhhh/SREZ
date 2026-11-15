package storage_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"srez/internal/entities"
	"srez/internal/storage"
	"srez/internal/usecase"
)

// Участники гонки имеют отдельные пулы по одному соединению и известные PID.
func auditConnection(t *testing.T, db *sqlx.DB) (*sqlx.DB, int) {
	t.Helper()
	var schema string
	pvmCheck(t, db.Get(&schema, `SELECT current_schema()`))
	u, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	pvmCheck(t, err)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	conn, err := sqlx.Connect("postgres", u.String())
	pvmCheck(t, err)
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)
	t.Cleanup(func() { conn.Close() })
	var pid int
	pvmCheck(t, conn.Get(&pid, `SELECT pg_backend_pid()`))
	return conn, pid
}
func auditWaitBlocked(t *testing.T, ctx context.Context, db *sqlx.DB, holder, waiter int) {
	t.Helper()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		var waiting bool
		pvmCheck(t, db.GetContext(ctx, &waiting, `SELECT $1=ANY(pg_blocking_pids($2))`, holder, waiter))
		if waiting {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("operation never waited on the expected transaction")
		case <-tick.C:
		}
	}
}

type auditGate struct{ ready, resume chan struct{} }

func newAuditGate() *auditGate { return &auditGate{make(chan struct{}), make(chan struct{})} }
func (g *auditGate) wait(ctx context.Context) error {
	close(g.ready)
	select {
	case <-g.resume:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type auditOrderGate struct {
	usecase.OrderRepository
	gate   *auditGate
	before bool
}

func (r auditOrderGate) WithinCheckout(ctx context.Context, uid int64, fn func(usecase.CheckoutTx) error) error {
	return r.OrderRepository.WithinCheckout(ctx, uid, func(tx usecase.CheckoutTx) error {
		if r.before {
			if err := r.gate.wait(ctx); err != nil {
				return err
			}
		}
		if err := fn(tx); err != nil {
			return err
		}
		if !r.before {
			return r.gate.wait(ctx)
		}
		return nil
	})
}

type auditCartGate struct {
	usecase.ShoppingRepository
	gate *auditGate
}

func (r auditCartGate) WithinCart(ctx context.Context, uid int64, fn func(usecase.CartTx) error) error {
	return r.ShoppingRepository.WithinCart(ctx, uid, func(tx usecase.CartTx) error {
		if err := fn(tx); err != nil {
			return err
		}
		return r.gate.wait(ctx)
	})
}
func auditStock(t *testing.T, f pvmFixture, want int64) {
	t.Helper()
	var stock int64
	pvmCheck(t, f.db.Get(&stock, `SELECT stock FROM product_variants WHERE id=$1`, f.variant))
	if stock != want {
		t.Fatalf("inventory: got %d want %d", stock, want)
	}
}
func auditSetStock(t *testing.T, f pvmFixture, stock int64) {
	t.Helper()
	_, err := f.m.ChangeVariant(context.Background(), f.owner, f.product, f.color, f.variant, usecase.VariantPatch{Stock: &stock})
	pvmCheck(t, err)
}

func TestPostgresAuditCartAndCheckoutSerialization(t *testing.T) {
	for _, first := range []string{"checkout", "cart"} {
		for _, action := range []string{"add", "patch", "delete"} {
			t.Run(first+"_then_"+action, func(t *testing.T) {
				f := newPVMFixture(t, true)
				auditSetStock(t, f, 10)
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				cartDB, cartPID := auditConnection(t, f.db)
				orderDB, orderPID := auditConnection(t, f.db)
				baseShop := storage.NewShopping(cartDB)
				baseOrders := storage.NewOrders(orderDB)
				shop := usecase.NewShopping(baseShop)
				orders := usecase.NewOrders(baseOrders)
				cart, err := shop.ChangeCart(ctx, f.owner, f.variant, 2, true)
				pvmCheck(t, err)
				id := cart.Items[0].ID
				gate := newAuditGate()
				if first == "checkout" {
					orders = usecase.NewOrders(auditOrderGate{baseOrders, gate, false})
				} else {
					shop = usecase.NewShopping(auditCartGate{baseShop, gate})
				}
				changed := make(chan error, 1)
				mutate := func() {
					var err error
					switch action {
					case "add":
						_, err = shop.ChangeCart(ctx, f.owner, f.variant, 1, true)
					case "patch":
						_, err = shop.ChangeCart(ctx, f.owner, id, 3, false)
					case "delete":
						err = shop.RemoveCartItem(ctx, f.owner, id)
					}
					changed <- err
				}
				type outcome struct {
					order *entities.Order
					err   error
				}
				checked := make(chan outcome, 1)
				checkout := func() { order, err := orders.Checkout(ctx, f.owner); checked <- outcome{order, err} }
				if first == "checkout" {
					go checkout()
					<-gate.ready
					go mutate()
					auditWaitBlocked(t, ctx, f.db, orderPID, cartPID)
				} else {
					go mutate()
					<-gate.ready
					go checkout()
					auditWaitBlocked(t, ctx, f.db, cartPID, orderPID)
				}
				close(gate.resume)
				mutationErr := <-changed
				result := <-checked
				after, err := usecase.NewShopping(baseShop).Cart(ctx, f.owner)
				pvmCheck(t, err)
				if first == "checkout" {
					pvmCheck(t, result.err)
					if result.order.Lines()[0].Quantity != 2 {
						t.Fatal("checkout snapshot lost its original quantity")
					}
					if action == "add" {
						pvmCheck(t, mutationErr)
						if len(after.Items) != 1 || after.Items[0].Quantity != 1 {
							t.Fatal("accepted add after checkout disappeared")
						}
					} else if !errors.Is(mutationErr, usecase.ErrResourceHidden) || len(after.Items) != 0 {
						t.Fatal("old cart line survived checkout", mutationErr)
					}
				} else {
					pvmCheck(t, mutationErr)
					if action == "delete" {
						if !errors.Is(result.err, usecase.ErrEmptyCart) || count(t, f.db, "orders") != 0 {
							t.Fatal(result.err)
						}
					} else {
						pvmCheck(t, result.err)
						if result.order.Lines()[0].Quantity != 3 {
							t.Fatal("accepted cart update was lost")
						}
					}
					if len(after.Items) != 0 {
						t.Fatal("purchased cart was not cleared")
					}
				}
				expected := 1
				if first == "cart" && action == "delete" {
					expected = 0
				}
				if count(t, f.db, "orders") != expected || count(t, f.db, "order_items") != expected {
					t.Fatal("unexpected order rows after serialized operations")
				}
				wantStock := int64(10)
				if result.order != nil && result.err == nil {
					wantStock -= result.order.Lines()[0].Quantity
				}
				auditStock(t, f, wantStock)
			})
		}
	}
}
func TestPostgresAuditEmptyCartHasSameSerializationBoundary(t *testing.T) {
	f := newPVMFixture(t, true)
	auditSetStock(t, f, 10)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	a, pidA := auditConnection(t, f.db)
	b, pidB := auditConnection(t, f.db)
	gate := newAuditGate()
	orders := usecase.NewOrders(auditOrderGate{storage.NewOrders(a), gate, true})
	shop := usecase.NewShopping(storage.NewShopping(b))
	checked := make(chan error, 1)
	go func() { _, err := orders.Checkout(ctx, f.owner); checked <- err }()
	<-gate.ready
	added := make(chan error, 1)
	go func() { _, err := shop.ChangeCart(ctx, f.owner, f.variant, 1, true); added <- err }()
	auditWaitBlocked(t, ctx, f.db, pidA, pidB)
	close(gate.resume)
	if err := <-checked; !errors.Is(err, usecase.ErrEmptyCart) {
		t.Fatal(err)
	}
	pvmCheck(t, <-added)
	cart, err := shop.Cart(ctx, f.owner)
	pvmCheck(t, err)
	if len(cart.Items) != 1 || count(t, f.db, "orders") != 0 {
		t.Fatal("newly populated cart was cleared by empty checkout")
	}
	auditStock(t, f, 10)
}

type auditManagementGate struct {
	usecase.ManagementRepository
	gate *auditGate
}

func (r auditManagementGate) WithinBrand(ctx context.Context, id int64, fn func(usecase.ManagementTx, *entities.Brand) error) error {
	return r.ManagementRepository.WithinBrand(ctx, id, func(tx usecase.ManagementTx, b *entities.Brand) error {
		if err := fn(tx, b); err != nil {
			return err
		}
		return r.gate.wait(ctx)
	})
}
func TestPostgresAuditCheckoutReReadsCatalogAfterManagement(t *testing.T) {
	for _, change := range []string{"price", "stock", "publication", "brand", "variant"} {
		t.Run(change, func(t *testing.T) {
			f := newPVMFixture(t, true)
			auditSetStock(t, f, 10)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, err := f.m.CreateVariant(ctx, f.owner, f.product, f.color, "M", 10, "")
			pvmCheck(t, err)
			_, err = usecase.NewShopping(storage.NewShopping(f.db)).ChangeCart(ctx, f.owner, f.variant, 1, true)
			pvmCheck(t, err)
			a, pidA := auditConnection(t, f.db)
			b, pidB := auditConnection(t, f.db)
			gate := newAuditGate()
			m := usecase.NewManagement(auditManagementGate{storage.NewManagement(a), gate})
			changed := make(chan error, 1)
			go func() {
				var err error
				switch change {
				case "price":
					v := int64(33333)
					_, err = m.ChangeProduct(ctx, f.owner, f.product, usecase.ProductPatch{PriceKopecks: &v}, "patch")
				case "stock":
					v := int64(0)
					_, err = m.ChangeVariant(ctx, f.owner, f.product, f.color, f.variant, usecase.VariantPatch{Stock: &v})
				case "publication":
					_, err = m.ChangeProduct(ctx, f.owner, f.product, usecase.ProductPatch{}, "hide")
				case "brand":
					_, err = m.ChangeBrand(ctx, f.owner, f.brand, usecase.BrandPatch{}, "unpublish")
				case "variant":
					err = m.DeleteVariant(ctx, f.owner, f.product, f.color, f.variant)
				}
				changed <- err
			}()
			<-gate.ready
			type outcome struct {
				order *entities.Order
				err   error
			}
			checked := make(chan outcome, 1)
			go func() {
				order, err := usecase.NewOrders(storage.NewOrders(b)).Checkout(ctx, f.owner)
				checked <- outcome{order, err}
			}()
			auditWaitBlocked(t, ctx, f.db, pidA, pidB)
			close(gate.resume)
			pvmCheck(t, <-changed)
			r := <-checked
			if change == "price" {
				pvmCheck(t, r.err)
				if r.order.TotalKopecks() != 33333 || count(t, f.db, "order_items") != 1 {
					t.Fatal("stale price")
				}
				auditStock(t, f, 9)
			} else {
				want := usecase.ErrCartUnavailable
				if change == "stock" {
					want = usecase.ErrInsufficientStock
				}
				if !errors.Is(r.err, want) || count(t, f.db, "orders") != 0 || count(t, f.db, "order_items") != 0 || count(t, f.db, "cart_items") != 1 {
					t.Fatal("invalid catalog state accepted", r.err)
				}
			}
		})
	}
}

// Отмена контекста после всех записей, но до commit, должна откатить заказ.
type auditCancelContext struct {
	usecase.OrderRepository
	cancel context.CancelFunc
}

func (r auditCancelContext) WithinCheckout(ctx context.Context, uid int64, fn func(usecase.CheckoutTx) error) error {
	return r.OrderRepository.WithinCheckout(ctx, uid, func(tx usecase.CheckoutTx) error {
		if err := fn(tx); err != nil {
			return err
		}
		r.cancel()
		return nil
	})
}
func TestPostgresAuditContextCancellationRollsBackCheckout(t *testing.T) {
	f := newPVMFixture(t, true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	shop := usecase.NewShopping(storage.NewShopping(f.db))
	_, err := shop.ChangeCart(ctx, f.owner, f.variant, 1, true)
	pvmCheck(t, err)
	before, err := shop.Cart(ctx, f.owner)
	pvmCheck(t, err)
	order, err := usecase.NewOrders(auditCancelContext{storage.NewOrders(f.db), cancel}).Checkout(ctx, f.owner)
	if order != nil || (!errors.Is(err, context.Canceled) && !errors.Is(err, sql.ErrTxDone)) {
		t.Fatal("cancelled transaction returned success", err)
	}
	after, err := shop.Cart(context.Background(), f.owner)
	pvmCheck(t, err)
	if !reflect.DeepEqual(before, after) || count(t, f.db, "orders") != 0 || count(t, f.db, "order_items") != 0 {
		t.Fatal("cancelled checkout committed")
	}
	auditStock(t, f, 1)
}
func TestPostgresAuditConcurrentQuantityBoundAndMoneyOverflow(t *testing.T) {
	f := newPVMFixture(t, true)
	auditSetStock(t, f, 200)
	ctx := context.Background()
	shop := usecase.NewShopping(storage.NewShopping(f.db))
	_, err := shop.ChangeCart(ctx, f.owner, f.variant, 99, true)
	pvmCheck(t, err)
	start := make(chan struct{})
	results := make(chan error, 4)
	for range 4 {
		go func() { <-start; _, err := shop.ChangeCart(ctx, f.owner, f.variant, 1, true); results <- err }()
	}
	close(start)
	ok := 0
	for range 4 {
		err := <-results
		if err == nil {
			ok++
		} else if !errors.Is(err, entities.ErrInvalidValue) {
			t.Fatal(err)
		}
	}
	cart, err := shop.Cart(ctx, f.owner)
	pvmCheck(t, err)
	if ok != 1 || len(cart.Items) != 1 || cart.Items[0].Quantity != 100 {
		t.Fatal("concurrent quantity exceeded bound")
	}
	auditStock(t, f, 200)
	// Разрешённая каталогу int64-цена не должна переполнять суммы корзины.
	price := int64(9223372036854775807)
	_, err = f.m.ChangeProduct(ctx, f.owner, f.product, usecase.ProductPatch{PriceKopecks: &price}, "patch")
	pvmCheck(t, err)
	if _, err = shop.Cart(ctx, f.owner); !errors.Is(err, entities.ErrMoneyOverflow) {
		t.Fatal(err)
	}
	if _, err = usecase.NewOrders(storage.NewOrders(f.db)).Checkout(ctx, f.owner); !errors.Is(err, entities.ErrMoneyOverflow) {
		t.Fatal(err)
	}
	if count(t, f.db, "orders") != 0 || count(t, f.db, "cart_items") != 1 {
		t.Fatal("overflow changed cart/order")
	}
	pvmCheck(t, shop.RemoveCartItem(ctx, f.owner, cart.Items[0].ID))
	cart, err = shop.ChangeCart(ctx, f.owner, f.variant, 1, true)
	pvmCheck(t, err)
	second, err := f.m.CreateVariant(ctx, f.owner, f.product, f.color, "M", 1, "")
	pvmCheck(t, err)
	if _, err = shop.ChangeCart(ctx, f.owner, second.ID(), 1, true); !errors.Is(err, entities.ErrMoneyOverflow) {
		t.Fatal("cart total overflow accepted", err)
	}
	var lines int
	pvmCheck(t, f.db.Get(&lines, `SELECT count(*) FROM cart_items WHERE user_id=$1`, f.owner))
	if lines != 1 {
		t.Fatal("overflowing add committed")
	}
	pvmCheck(t, shop.RemoveCartItem(ctx, f.owner, cart.Items[0].ID))
	auditStock(t, f, 200)
}

func TestPostgresAuditConcurrentCancellationReleasesOnce(t *testing.T) {
	f := newPVMFixture(t, true)
	ctx := context.Background()
	shop := usecase.NewShopping(storage.NewShopping(f.db))
	orders := usecase.NewOrders(storage.NewOrders(f.db))
	_, err := shop.ChangeCart(ctx, f.owner, f.variant, 1, true)
	pvmCheck(t, err)
	order, err := orders.Checkout(ctx, f.owner)
	pvmCheck(t, err)
	a, _ := auditConnection(t, f.db)
	b, _ := auditConnection(t, f.db)
	start := make(chan struct{})
	results := make(chan *entities.Order, 2)
	errs := make(chan error, 2)
	for _, db := range []*sqlx.DB{a, b} {
		go func() {
			<-start
			got, err := usecase.NewOrders(storage.NewOrders(db)).Cancel(ctx, f.owner, order.ID())
			results <- got
			errs <- err
		}()
	}
	close(start)
	pvmCheck(t, <-errs)
	pvmCheck(t, <-errs)
	first, second := <-results, <-results
	if first.Status() != "cancelled" || !reflect.DeepEqual(first.State(), second.State()) {
		t.Fatal("cancellation was not idempotent")
	}
	auditStock(t, f, 1)
	var n int
	pvmCheck(t, f.db.Get(&n, `SELECT count(*) FROM orders WHERE status='cancelled' AND cancelled_at IS NOT NULL`))
	if n != 1 || count(t, f.db, "order_items") != 1 || count(t, f.db, "cart_items") != 0 {
		t.Fatal("cancellation changed snapshots/cart")
	}
}

func TestPostgresAuditRepeatedReservationCancellationDoesNotInflateStock(t *testing.T) {
	f := newPVMFixture(t, true)
	ctx := context.Background()
	shop := usecase.NewShopping(storage.NewShopping(f.db))
	orders := usecase.NewOrders(storage.NewOrders(f.db))
	for range 5 {
		_, err := shop.ChangeCart(ctx, f.owner, f.variant, 1, true)
		pvmCheck(t, err)
		order, err := orders.Checkout(ctx, f.owner)
		pvmCheck(t, err)
		auditStock(t, f, 0)
		_, err = orders.Cancel(ctx, f.owner, order.ID())
		pvmCheck(t, err)
		auditStock(t, f, 1)
	}
	if count(t, f.db, "orders") != 5 || count(t, f.db, "order_items") != 5 {
		t.Fatal(fmt.Errorf("history was lost"))
	}
}

func TestPostgresAuditCartMutationsRaceCatalogDeletion(t *testing.T) {
	for _, kind := range []string{"variant", "color"} {
		for _, action := range []string{"add", "patch", "delete"} {
			t.Run(kind+"_"+action, func(t *testing.T) {
				f := newPVMFixture(t, true)
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				var unrelated int64
				if kind == "color" {
					_, err := f.m.ChangeProduct(ctx, f.owner, f.product, usecase.ProductPatch{}, "hide")
					pvmCheck(t, err)
					c, err := f.m.CreateColor(ctx, f.owner, f.product, "White")
					pvmCheck(t, err)
					_, err = f.m.CreateImage(ctx, f.owner, f.product, c.ID(), "https://example.com/white.jpg", nil)
					pvmCheck(t, err)
					v, err := f.m.CreateVariant(ctx, f.owner, f.product, c.ID(), "M", 10, "")
					pvmCheck(t, err)
					unrelated = v.ID()
					_, err = f.m.ChangeProduct(ctx, f.owner, f.product, usecase.ProductPatch{}, "publish")
					pvmCheck(t, err)
				} else {
					v, err := f.m.CreateVariant(ctx, f.owner, f.product, f.color, "M", 10, "")
					pvmCheck(t, err)
					unrelated = v.ID()
				}
				other := managedUser(t, usecase.NewUsers(storage.NewUsers(f.db)), "delete_race_other")
				base := usecase.NewShopping(storage.NewShopping(f.db))
				cart, err := base.ChangeCart(ctx, f.owner, f.variant, 1, true)
				pvmCheck(t, err)
				id := cart.Items[0].ID
				_, err = base.ChangeCart(ctx, other, unrelated, 1, true)
				pvmCheck(t, err)
				a, pidA := auditConnection(t, f.db)
				b, pidB := auditConnection(t, f.db)
				gate := newAuditGate()
				m := usecase.NewManagement(auditManagementGate{storage.NewManagement(a), gate})
				removed := make(chan error, 1)
				go func() {
					if kind == "color" {
						removed <- m.DeleteColor(ctx, f.owner, f.product, f.color)
					} else {
						removed <- m.DeleteVariant(ctx, f.owner, f.product, f.color, f.variant)
					}
				}()
				<-gate.ready
				shop := usecase.NewShopping(storage.NewShopping(b))
				changed := make(chan error, 1)
				go func() {
					var err error
					switch action {
					case "add":
						_, err = shop.ChangeCart(ctx, f.owner, f.variant, 1, true)
					case "patch":
						_, err = shop.ChangeCart(ctx, f.owner, id, 1, false)
					case "delete":
						err = shop.RemoveCartItem(ctx, f.owner, id)
					}
					changed <- err
				}()
				auditWaitBlocked(t, ctx, f.db, pidA, pidB)
				close(gate.resume)
				pvmCheck(t, <-removed)
				err = <-changed
				if action == "delete" {
					pvmCheck(t, err)
				} else {
					want := usecase.ErrResourceHidden
					if action == "patch" {
						want = usecase.ErrCartUnavailable
					}
					if !errors.Is(err, want) {
						t.Fatal("unexpected deletion race result", err)
					}
				}
				own, err := base.Cart(ctx, f.owner)
				pvmCheck(t, err)
				if action == "delete" {
					if len(own.Items) != 0 {
						t.Fatal("line was not removed")
					}
				} else if len(own.Items) != 1 || own.Items[0].ID != id || own.Items[0].VariantID != nil || own.Items[0].Quantity != 1 {
					t.Fatal("referenced cart line silently lost")
				}
				foreign, err := base.Cart(ctx, other)
				pvmCheck(t, err)
				if len(foreign.Items) != 1 || foreign.Items[0].VariantID == nil || *foreign.Items[0].VariantID != unrelated || foreign.Items[0].Quantity != 1 {
					t.Fatal("unrelated buyer data changed")
				}
				var stock int64
				pvmCheck(t, f.db.Get(&stock, `SELECT stock FROM product_variants WHERE id=$1`, unrelated))
				if stock != 10 {
					t.Fatal("deletion/cart operation changed unrelated stock")
				}
			})
		}
	}
}

func TestPostgresAuditCancellationReChecksDeadlineAfterOrderLock(t *testing.T) {
	f := newPVMFixture(t, true)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := usecase.NewShopping(storage.NewShopping(f.db)).ChangeCart(ctx, f.owner, f.variant, 1, true)
	pvmCheck(t, err)
	order, err := usecase.NewOrders(storage.NewOrders(f.db)).Checkout(ctx, f.owner)
	pvmCheck(t, err)
	a, pidA := auditConnection(t, f.db)
	b, pidB := auditConnection(t, f.db)
	guard, err := a.BeginTxx(ctx, nil)
	pvmCheck(t, err)
	defer guard.Rollback()
	_, err = guard.ExecContext(ctx, `UPDATE orders SET created_at=now()-interval '16 minutes',cancel_before=now()-interval '1 minute' WHERE id=$1 AND user_id=$2`, order.ID(), f.owner)
	pvmCheck(t, err)
	done := make(chan error, 1)
	go func() {
		_, err := usecase.NewOrders(storage.NewOrders(b)).Cancel(ctx, f.owner, order.ID())
		done <- err
	}()
	auditWaitBlocked(t, ctx, f.db, pidA, pidB)
	pvmCheck(t, guard.Commit())
	if err = <-done; !errors.Is(err, entities.ErrOrderCancellationClosed) {
		t.Fatal("stale cancellation deadline accepted", err)
	}
	var status string
	pvmCheck(t, f.db.Get(&status, `SELECT status FROM orders WHERE id=$1`, order.ID()))
	if status != "unpaid" {
		t.Fatal(status)
	}
	auditStock(t, f, 0)
}
