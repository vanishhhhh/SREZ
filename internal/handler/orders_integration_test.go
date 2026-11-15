package handler

import (
	"context"
	"fmt"
	"reflect"
	"srez/internal/devseed"
	"srez/internal/storage"
	"srez/internal/usecase"
	"testing"
)

func TestPostgresHTTPCheckoutAndHistory(t *testing.T) {
	h, db := newPVMHTTP(t)
	h.SetShopping(usecase.NewShopping(storage.NewShopping(db)))
	h.SetOrders(usecase.NewOrders(storage.NewOrders(db)))
	buyer, _ := pvmAccount(t, h, "checkout_buyer")
	other, _ := pvmAccount(t, h, "checkout_other")
	if err := devseed.Apply(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if list := pvmDecode[[]orderResponse](t, buyer.call("GET", "/orders", "", 200)); len(list) != 0 {
		t.Fatal(list)
	}
	buyer.call("POST", "/orders", `{}`, 409)
	products := pvmDecode[[]productCardResponse](t, buyer.call("GET", "/products", "", 200))
	for _, p := range products {
		detail, err := storage.NewProducts(db).GetPublic(context.Background(), p.ID)
		if err != nil {
			t.Fatal(err)
		}
		buyer.call("POST", "/cart/items", fmt.Sprintf(`{"product_variant_id":%d,"quantity":1}`, detail.Publication.Variants[0].ID()), 200)
	}
	cart := pvmDecode[cartResponse](t, buyer.call("GET", "/cart", "", 200))
	w := buyer.call("POST", "/orders", `{}`, 201)
	order := pvmDecode[orderResponse](t, w)
	if order.Status != "unpaid" || len(order.Items) != 2 || order.TotalKopecks != cart.TotalKopecks || order.Items[0].BrandID == order.Items[1].BrandID || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(order)
	}
	path := fmt.Sprintf("/orders/%d", order.ID)
	got := pvmDecode[orderResponse](t, buyer.call("GET", path, "", 200))
	list := pvmDecode[[]orderResponse](t, buyer.call("GET", "/orders", "", 200))
	if !reflect.DeepEqual(order, got) || len(list) != 1 || !reflect.DeepEqual(order, list[0]) {
		t.Fatal("history differs")
	}
	other.call("GET", path, "", 404)
	if list := pvmDecode[[]orderResponse](t, other.call("GET", "/orders", "", 200)); len(list) != 0 {
		t.Fatal("cross-user history")
	}
	buyer.call("GET", "/orders/0", "", 400)
	buyer.call("GET", "/orders/9223372036854775808", "", 400)
	buyer.call("POST", "/orders", `{}`, 409)
	cart = pvmDecode[cartResponse](t, buyer.call("GET", "/cart", "", 200))
	if len(cart.Items) != 0 || cart.TotalKopecks != 0 {
		t.Fatal(cart)
	}
}
