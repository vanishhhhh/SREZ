package handler

import (
	"context"
	"fmt"
	"srez/internal/devseed"
	"srez/internal/storage"
	"srez/internal/usecase"
	"testing"
)

func TestPostgresHTTPShopping(t *testing.T) {
	h, db := newPVMHTTP(t)
	h.SetShopping(usecase.NewShopping(storage.NewShopping(db)))
	buyer, _ := pvmAccount(t, h, "shop_buyer")
	other, _ := pvmAccount(t, h, "shop_other")
	if err := devseed.Apply(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	products := pvmDecode[[]productCardResponse](t, buyer.call("GET", "/products", "", 200))
	detail, err := storage.NewProducts(db).GetPublic(context.Background(), products[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	variant := detail.Publication.Variants[0].ID()
	path := fmt.Sprintf("/favorites/%d", products[0].ID)
	buyer.call("POST", path, "", 204)
	buyer.call("POST", path, "", 204)
	w := buyer.call("GET", "/favorites", "", 200)
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("private cache")
	}
	favorites := pvmDecode[[]favoriteResponse](t, w)
	if len(favorites) != 1 || favorites[0].Product == nil {
		t.Fatal(favorites)
	}
	if got := pvmDecode[[]favoriteResponse](t, other.call("GET", "/favorites", "", 200)); len(got) != 0 {
		t.Fatal("cross-user favorites")
	}
	cart := pvmDecode[cartResponse](t, buyer.call("POST", "/cart/items", fmt.Sprintf(`{"product_variant_id":%d,"quantity":1}`, variant), 200))
	if len(cart.Items) != 1 || cart.Items[0].Product == nil || cart.TotalKopecks != products[0].PriceKopecks {
		t.Fatal(cart)
	}
	item := fmt.Sprintf("/cart/items/%d", cart.Items[0].ID)
	other.call("PATCH", item, `{"quantity":1}`, 404)
	other.call("DELETE", item, "", 404)
	cart = pvmDecode[cartResponse](t, buyer.call("PATCH", item, `{"quantity":2}`, 200))
	if cart.TotalKopecks != 2*products[0].PriceKopecks {
		t.Fatal(cart)
	}
	buyer.call("DELETE", item, "", 204)
	buyer.call("DELETE", path, "", 204)
	buyer.call("DELETE", path, "", 204)
	cart = pvmDecode[cartResponse](t, buyer.call("GET", "/cart", "", 200))
	if len(cart.Items) != 0 || cart.TotalKopecks != 0 {
		t.Fatal(cart)
	}
}
