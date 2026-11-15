package storage_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"srez/internal/entities"
	"srez/internal/storage"
	"srez/internal/usecase"
)

func TestPostgresShoppingFavoritesAndCart(t *testing.T) {
	f := newPVMFixture(t, true)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	shop := usecase.NewShopping(storage.NewShopping(f.db))
	other := managedUser(t, usecase.NewUsers(storage.NewUsers(f.db)), "buyer_other")
	stock := int64(100)
	_, err := f.m.ChangeVariant(ctx, f.owner, f.product, f.color, f.variant, usecase.VariantPatch{Stock: &stock})
	pvmCheck(t, err)
	start := make(chan struct{})
	errs := make(chan error, 12)
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; errs <- shop.AddFavorite(ctx, f.owner, f.product) }()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		pvmCheck(t, err)
	}
	if count(t, f.db, "favorites") != 1 {
		t.Fatal("duplicate favorites")
	}
	favorites, err := shop.Favorites(ctx, f.owner)
	pvmCheck(t, err)
	if len(favorites) != 1 || favorites[0].Product == nil || favorites[0].Product.Product.ID() != f.product {
		t.Fatal(favorites)
	}
	favorites, err = shop.Favorites(ctx, other)
	pvmCheck(t, err)
	if len(favorites) != 0 {
		t.Fatal("cross-user favorites")
	}
	pvmCheck(t, shop.RemoveFavorite(ctx, other, f.product))
	if count(t, f.db, "favorites") != 1 {
		t.Fatal("other user deleted favorite")
	}
	start = make(chan struct{})
	errs = make(chan error, 12)
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := shop.ChangeCart(ctx, f.owner, f.variant, 1, true)
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		pvmCheck(t, err)
	}
	cart, err := shop.Cart(ctx, f.owner)
	pvmCheck(t, err)
	if len(cart.Items) != 1 || cart.Items[0].Quantity != 12 || cart.TotalKopecks != 120000 {
		t.Fatal(cart)
	}
	id := cart.Items[0].ID
	if _, err = shop.ChangeCart(ctx, other, id, 2, false); !errors.Is(err, usecase.ErrResourceHidden) {
		t.Fatal("cross-user patch", err)
	}
	if err = shop.RemoveCartItem(ctx, other, id); !errors.Is(err, usecase.ErrResourceHidden) {
		t.Fatal("cross-user delete", err)
	}
	for _, q := range []int64{0, -1, 101} {
		if _, err = shop.ChangeCart(ctx, f.owner, f.variant, q, true); !errors.Is(err, entities.ErrInvalidValue) {
			t.Fatal(q, err)
		}
	}
	if _, err = shop.ChangeCart(ctx, f.owner, 9223372036854775807, 1, true); !errors.Is(err, usecase.ErrResourceHidden) {
		t.Fatal(err)
	}
	if _, err = shop.ChangeCart(ctx, f.owner, f.variant, 89, true); !errors.Is(err, entities.ErrInvalidValue) {
		t.Fatal("sum bound", err)
	}
	stock = 11
	_, err = f.m.ChangeVariant(ctx, f.owner, f.product, f.color, f.variant, usecase.VariantPatch{Stock: &stock})
	pvmCheck(t, err)
	cart, err = shop.Cart(ctx, f.owner)
	pvmCheck(t, err)
	if cart.TotalKopecks != 0 || cart.Items[0].Availability != "insufficient_stock" {
		t.Fatal(cart)
	}
	if _, err = shop.ChangeCart(ctx, f.owner, f.variant, 1, true); !errors.Is(err, usecase.ErrInsufficientStock) {
		t.Fatal(err)
	}
	cart, err = shop.ChangeCart(ctx, f.owner, id, 2, false)
	pvmCheck(t, err)
	if cart.Items[0].Quantity != 2 || cart.TotalKopecks != 20000 {
		t.Fatal(cart)
	}
	var gotStock int64
	pvmCheck(t, f.db.Get(&gotStock, `SELECT stock FROM product_variants WHERE id=$1`, f.variant))
	if gotStock != 11 {
		t.Fatal("cart changed stock", gotStock)
	}
	_, err = f.m.ChangeProduct(ctx, f.owner, f.product, usecase.ProductPatch{}, "hide")
	pvmCheck(t, err)
	cart, err = shop.Cart(ctx, f.owner)
	pvmCheck(t, err)
	favorites, err = shop.Favorites(ctx, f.owner)
	pvmCheck(t, err)
	if cart.Items[0].Product != nil || cart.TotalKopecks != 0 || favorites[0].Product != nil {
		t.Fatal("hidden fields exposed")
	}
	if err = shop.AddFavorite(ctx, other, f.product); !errors.Is(err, usecase.ErrResourceHidden) {
		t.Fatal(err)
	}
	if _, err = shop.ChangeCart(ctx, f.owner, f.variant, 1, true); !errors.Is(err, usecase.ErrResourceHidden) {
		t.Fatal(err)
	}
	pvmCheck(t, shop.RemoveFavorite(ctx, f.owner, f.product))
	pvmCheck(t, shop.RemoveFavorite(ctx, f.owner, f.product))
	pvmCheck(t, shop.RemoveCartItem(ctx, f.owner, id))
	if count(t, f.db, "cart_items") != 0 {
		t.Fatal("unavailable line not removable")
	}
}
func TestPostgresShoppingOutOfStockAndSuspended(t *testing.T) {
	f := newPVMFixture(t, true)
	ctx := context.Background()
	shop := usecase.NewShopping(storage.NewShopping(f.db))
	_, err := shop.ChangeCart(ctx, f.owner, f.variant, 1, true)
	pvmCheck(t, err)
	stock := int64(0)
	_, err = f.m.ChangeVariant(ctx, f.owner, f.product, f.color, f.variant, usecase.VariantPatch{Stock: &stock})
	pvmCheck(t, err)
	pvmCheck(t, shop.AddFavorite(ctx, f.owner, f.product))
	favorites, err := shop.Favorites(ctx, f.owner)
	pvmCheck(t, err)
	if favorites[0].Product == nil {
		t.Fatal("out of stock favorite hidden")
	}
	exec(t, f.db, `UPDATE brands SET status='suspended' WHERE id=$1`, f.brand)
	favorites, err = shop.Favorites(ctx, f.owner)
	pvmCheck(t, err)
	cart, err := shop.Cart(ctx, f.owner)
	pvmCheck(t, err)
	if favorites[0].Product != nil || cart.Items[0].Product != nil {
		t.Fatal("suspended fields exposed")
	}
	if err = shop.AddFavorite(ctx, f.owner, f.product); !errors.Is(err, usecase.ErrResourceHidden) {
		t.Fatal(err)
	}
}
func TestPostgresShoppingDeletionTombstonesAndRollback(t *testing.T) {
	for _, kind := range []string{"variant", "color"} {
		t.Run(kind, func(t *testing.T) {
			f := newPVMFixture(t, true)
			ctx := context.Background()
			shop := usecase.NewShopping(storage.NewShopping(f.db))
			cart, err := shop.ChangeCart(ctx, f.owner, f.variant, 1, true)
			pvmCheck(t, err)
			id := cart.Items[0].ID
			remove := func() error {
				if kind == "variant" {
					return f.m.DeleteVariant(ctx, f.owner, f.product, f.color, f.variant)
				}
				return f.m.DeleteColor(ctx, f.owner, f.product, f.color)
			}
			if err = remove(); !errors.Is(err, entities.ErrNotReady) {
				t.Fatal(err)
			}
			cart, err = shop.Cart(ctx, f.owner)
			pvmCheck(t, err)
			if cart.Items[0].VariantID == nil || cart.Items[0].Product == nil {
				t.Fatal("failed delete detached cart")
			}
			_, err = f.m.ChangeProduct(ctx, f.owner, f.product, usecase.ProductPatch{}, "hide")
			pvmCheck(t, err)
			pvmCheck(t, remove())
			cart, err = shop.Cart(ctx, f.owner)
			pvmCheck(t, err)
			if len(cart.Items) != 1 || cart.Items[0].ID != id || cart.Items[0].VariantID != nil || cart.Items[0].Product != nil || cart.Items[0].Availability != "unavailable" {
				t.Fatal(cart)
			}
			if _, err = shop.ChangeCart(ctx, f.owner, id, 1, false); !errors.Is(err, usecase.ErrCartUnavailable) {
				t.Fatal(err)
			}
			pvmCheck(t, shop.RemoveCartItem(ctx, f.owner, id))
		})
	}
}
