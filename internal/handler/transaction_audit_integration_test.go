package handler

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"srez/internal/devseed"
	"srez/internal/storage"
	"srez/internal/usecase"
)

// Сравниваются сами строки, а не только их количество: отказ не должен менять
// владельца, количество, снимки, статус заказа или остатки.
func httpAuditState(t *testing.T, db *sqlx.DB) string {
	t.Helper()
	var state string
	err := db.Get(&state, `SELECT jsonb_build_object(
 'favorites',(SELECT COALESCE(jsonb_agg(to_jsonb(f) ORDER BY user_id,product_id),'[]') FROM favorites f),
 'cart',(SELECT COALESCE(jsonb_agg(to_jsonb(c) ORDER BY id),'[]') FROM cart_items c),
 'orders',(SELECT COALESCE(jsonb_agg(to_jsonb(o) ORDER BY id),'[]') FROM orders o),
 'lines',(SELECT COALESCE(jsonb_agg(to_jsonb(i) ORDER BY id),'[]') FROM order_items i),
 'reservations',(SELECT COALESCE(jsonb_agg(to_jsonb(r) ORDER BY order_id,product_variant_id),'[]') FROM order_reservations r),
 'inventory',(SELECT COALESCE(jsonb_agg(to_jsonb(v) ORDER BY id),'[]') FROM product_variants v))::text`)
	if err != nil {
		t.Fatal(err)
	}
	return state
}
func auditHTTP(t *testing.T) (*Handler, *sqlx.DB, pvmHTTPClient, pvmHTTPClient, int64, int64) {
	t.Helper()
	h, db := newPVMHTTP(t)
	h.SetShopping(usecase.NewShopping(storage.NewShopping(db)))
	h.SetOrders(usecase.NewOrders(storage.NewOrders(db)))
	buyer, _ := pvmAccount(t, h, "audit_buyer")
	other, _ := pvmAccount(t, h, "audit_other")
	if err := devseed.Apply(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	products := pvmDecode[[]productCardResponse](t, buyer.call("GET", "/products", "", 200))
	detail, err := storage.NewProducts(db).GetPublic(context.Background(), products[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	return h, db, buyer, other, products[0].ID, detail.Publication.Variants[0].ID()
}
func TestPostgresHTTPAuditRejectUnexpectedBodyOnBodylessRoutes(t *testing.T) {
	_, db, buyer, _, product, variant := auditHTTP(t)
	cart := pvmDecode[cartResponse](t, buyer.call("POST", "/cart/items", fmt.Sprintf(`{"product_variant_id":%d,"quantity":1}`, variant), 200))
	for _, r := range []struct{ method, path string }{
		{"POST", fmt.Sprintf("/favorites/%d", product)},
		{"DELETE", fmt.Sprintf("/favorites/%d", product)},
		{"DELETE", fmt.Sprintf("/cart/items/%d", cart.Items[0].ID)},
	} {
		for _, body := range []string{`{"user_id":999}`, `{"price_kopecks":1}`, `{"status":"paid"}`, `{}`, `not json`} {
			before := httpAuditState(t, db)
			req := httptest.NewRequest(r.method, r.path, strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+buyer.token)
			w := httptest.NewRecorder()
			buyer.h.router.ServeHTTP(w, req)
			changed := before != httpAuditState(t, db)
			if w.Code != 400 || changed {
				t.Fatalf("%s %s: status=%d, state_changed=%t", r.method, r.path, w.Code, changed)
			}
		}
	}
}
func TestPostgresHTTPAuditAuthorizationInputAndNoDisclosure(t *testing.T) {
	var logs bytes.Buffer
	oldOut, oldErr := gin.DefaultWriter, gin.DefaultErrorWriter
	gin.DefaultWriter, gin.DefaultErrorWriter = &logs, &logs
	t.Cleanup(func() { gin.DefaultWriter, gin.DefaultErrorWriter = oldOut, oldErr })
	h, db, buyer, other, product, variant := auditHTTP(t)
	favorite := fmt.Sprintf("/favorites/%d", product)
	buyer.call("POST", favorite, "", 204)
	buyer.call("POST", "/cart/items", fmt.Sprintf(`{"product_variant_id":%d,"quantity":1}`, variant), 200)
	order := pvmDecode[orderResponse](t, buyer.call("POST", "/orders", `{}`, 201))
	cart := pvmDecode[cartResponse](t, buyer.call("POST", "/cart/items", fmt.Sprintf(`{"product_variant_id":%d,"quantity":1}`, variant), 200))
	item := fmt.Sprintf("/cart/items/%d", cart.Items[0].ID)
	orderPath := fmt.Sprintf("/orders/%d", order.ID)
	routes := []struct{ method, path, body string }{
		{"GET", "/favorites", ""}, {"POST", favorite, ""}, {"DELETE", favorite, ""},
		{"GET", "/cart", ""}, {"POST", "/cart/items", fmt.Sprintf(`{"product_variant_id":%d,"quantity":1}`, variant)},
		{"PATCH", item, `{"quantity":2}`}, {"DELETE", item, ""},
		{"POST", "/orders", `{}`}, {"GET", "/orders", ""}, {"GET", orderPath, ""}, {"POST", orderPath + "/cancel", `{}`},
	}
	for _, r := range routes {
		for _, auth := range []string{"", "Bearer audit-invalid-token", "Basic audit-invalid-token"} {
			before := httpAuditState(t, db)
			req := httptest.NewRequest(r.method, r.path, strings.NewReader(r.body))
			req.Header.Set("Authorization", auth)
			req.AddCookie(&http.Cookie{Name: "srez_refresh_local", Value: "audit-cookie-is-not-bearer"})
			w := httptest.NewRecorder()
			h.router.ServeHTTP(w, req)
			if w.Code != 401 || w.Header().Get("Cache-Control") != "no-store" || before != httpAuditState(t, db) {
				t.Fatalf("unauthorized %s %s: %d", r.method, r.path, w.Code)
			}
		}
	}
	before := httpAuditState(t, db)
	if got := pvmDecode[[]favoriteResponse](t, other.call("GET", "/favorites", "", 200)); len(got) != 0 {
		t.Fatal("foreign favorites disclosed")
	}
	if got := pvmDecode[cartResponse](t, other.call("GET", "/cart", "", 200)); len(got.Items) != 0 {
		t.Fatal("foreign cart disclosed")
	}
	if got := pvmDecode[[]orderResponse](t, other.call("GET", "/orders", "", 200)); len(got) != 0 {
		t.Fatal("foreign order list disclosed")
	}
	for _, r := range []struct{ method, path, body string }{{"PATCH", item, `{"quantity":2}`}, {"DELETE", item, ""}, {"GET", orderPath, ""}, {"POST", orderPath + "/cancel", `{}`}} {
		w := other.call(r.method, r.path, r.body, 404)
		if strings.TrimSpace(w.Body.String()) != `{"error":"resource not found"}` || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("foreign resource response", w.Body.String())
		}
	}
	other.call("DELETE", favorite, "", 204)
	if before != httpAuditState(t, db) {
		t.Fatal("foreign access changed database")
	}
	for _, r := range []struct{ method, path, body string }{
		{"POST", "/cart/items", fmt.Sprintf(`{"product_variant_id":%d,"quantity":1,"user_id":999}`, variant)},
		{"POST", "/cart/items", fmt.Sprintf(`{"product_variant_id":%d,"quantity":1,"discount":100}`, variant)},
		{"POST", "/cart/items", fmt.Sprintf(`{"product_variant_id":%d,"quantity":1,"stock":999}`, variant)},
		{"POST", "/cart/items", `{"product_variant_id":9223372036854775808,"quantity":1}`},
		{"PATCH", item, `{"quantity":0}`}, {"PATCH", item, `{"quantity":-1}`}, {"PATCH", item, `{"quantity":101}`},
		{"PATCH", item, `{"quantity":9223372036854775807}`}, {"PATCH", item, `{"quantity":1.5}`}, {"PATCH", item, `{"quantity":"1"}`},
		{"PATCH", item, `{"quantity":1,"price_kopecks":0}`}, {"POST", "/orders", `{"user_id":999}`},
		{"POST", "/orders", `{"status":"paid"}`}, {"POST", "/orders", `{"payment_status":"paid"}`},
		{"POST", "/orders", `{"total_kopecks":1}`}, {"GET", "/orders/9223372036854775808", ""},
	} {
		buyer.call(r.method, r.path, r.body, 400)
		if before != httpAuditState(t, db) {
			t.Fatalf("invalid input changed database: %s %s", r.method, r.path)
		}
	}
	for _, secret := range []string{buyer.token, other.token, "audit-invalid-token", "audit_buyer@example.com", "local PVM test password 123"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatal("request/account secret leaked into logs")
		}
	}
}

func TestPostgresHTTPAuditCancellationOwnershipDeadlineAndReservationReturn(t *testing.T) {
	h, db, buyer, other, _, variant := auditHTTP(t)
	var stockBefore, stockAfter int64
	if err := db.Get(&stockBefore, `SELECT stock FROM product_variants WHERE id=$1`, variant); err != nil {
		t.Fatal(err)
	}
	buyer.call("POST", "/cart/items", fmt.Sprintf(`{"product_variant_id":%d,"quantity":1}`, variant), 200)
	order := pvmDecode[orderResponse](t, buyer.call("POST", "/orders", `{}`, 201))
	path := fmt.Sprintf("/orders/%d/cancel", order.ID)
	if order.CancelledAt != nil || !order.CancelBefore.Equal(order.CreatedAt.Add(15*time.Minute)) {
		t.Fatal("invalid cancellation deadline")
	}
	before := httpAuditState(t, db)
	guest := pvmHTTPClient{t: t, h: h}
	guest.call("POST", path, `{}`, 401)
	other.call("POST", path, `{}`, 404)
	for _, body := range []string{`{"user_id":999}`, `{"status":"paid"}`, `{"cancelled_at":"2026-01-01T00:00:00Z"}`, `{"refund":true}`} {
		buyer.call("POST", path, body, 400)
	}
	if before != httpAuditState(t, db) {
		t.Fatal("denied cancellation changed data")
	}
	first := pvmDecode[orderResponse](t, buyer.call("POST", path, `{}`, 200))
	second := pvmDecode[orderResponse](t, buyer.call("POST", path, `{}`, 200))
	if first.Status != "cancelled" || first.CancelledAt == nil || !reflect.DeepEqual(first, second) || !reflect.DeepEqual(first.Items, order.Items) || first.TotalKopecks != order.TotalKopecks {
		t.Fatal("cancellation not stable")
	}
	buyer.call("POST", "/cart/items", fmt.Sprintf(`{"product_variant_id":%d,"quantity":1}`, variant), 200)
	expired := pvmDecode[orderResponse](t, buyer.call("POST", "/orders", `{}`, 201))
	// now() одинаков в пределах одного UPDATE; CHECK сохраняет точные 15 минут.
	if _, err := db.Exec(`UPDATE orders SET created_at=now()-interval '16 minutes',cancel_before=now()-interval '1 minute' WHERE id=$1`, expired.ID); err != nil {
		t.Fatal(err)
	}
	before = httpAuditState(t, db)
	w := buyer.call("POST", fmt.Sprintf("/orders/%d/cancel", expired.ID), `{}`, 409)
	if !strings.Contains(w.Body.String(), "contact support") || before != httpAuditState(t, db) {
		t.Fatal("expired cancellation mutated data")
	}
	if _, err := usecase.NewOrders(storage.NewOrders(db)).Expire(context.Background(), time.Now().UTC(), 100); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&stockAfter, `SELECT stock FROM product_variants WHERE id=$1`, variant); err != nil {
		t.Fatal(err)
	}
	if stockAfter != stockBefore {
		t.Fatal("unpaid order/cancellation changed stock")
	}
}

func TestPostgresHTTPAuditDocumentedCheckoutRetryContract(t *testing.T) {
	h, db, buyer, other, _, variant := auditHTTP(t)
	var stockBefore int64
	if err := db.Get(&stockBefore, `SELECT stock FROM product_variants WHERE id=$1`, variant); err != nil {
		t.Fatal(err)
	}
	add := fmt.Sprintf(`{"product_variant_id":%d,"quantity":1}`, variant)
	buyer.call("POST", "/cart/items", add, 200)
	request := func(token, key string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/orders", strings.NewReader(`{}`))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Idempotency-Key", key)
		w := httptest.NewRecorder()
		h.router.ServeHTTP(w, r)
		return w
	}
	for i, pair := range [][]string{{"audit-key-one", "audit-key-two"}, {"audit-same-key", "audit-same-key"}} {
		if i > 0 {
			buyer.call("POST", "/cart/items", add, 200)
		}
		start := make(chan struct{})
		results := make(chan *httptest.ResponseRecorder, 2)
		for _, key := range pair {
			go func() { <-start; results <- request(buyer.token, key) }()
		}
		close(start)
		success, empty := 0, 0
		for range 2 {
			w := <-results
			switch w.Code {
			case 201:
				success++
			case 409:
				empty++
			default:
				t.Fatal(w.Code, w.Body.String())
			}
		}
		if success != 1 || empty != 1 {
			t.Fatal("same cart checked out twice")
		}
	}
	// Ключи не поддерживаются существующим контрактом: без новой корзины — 409.
	if w := request(buyer.token, "audit-key-one"); w.Code != 409 {
		t.Fatal(w.Code)
	}
	buyer.call("POST", "/cart/items", add, 200)
	if w := request(buyer.token, "audit-key-one"); w.Code != 201 {
		t.Fatal("new cart must follow documented contract", w.Code)
	}
	other.call("POST", "/cart/items", add, 200)
	if w := request(other.token, "audit-key-one"); w.Code != 201 {
		t.Fatal("unrelated buyer interfered", w.Code)
	}
	var orders, lines int
	if err := db.Get(&orders, `SELECT count(*) FROM orders`); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&lines, `SELECT count(*) FROM order_items`); err != nil {
		t.Fatal(err)
	}
	if orders != 4 || lines != 4 {
		t.Fatal("unexpected retry state", orders, lines)
	}
	var stockAfter int64
	if err := db.Get(&stockAfter, `SELECT stock FROM product_variants WHERE id=$1`, variant); err != nil {
		t.Fatal(err)
	}
	if stockBefore-4 != stockAfter {
		t.Fatal("retries reserved incorrect quantity")
	}
	var carts int
	if err := db.Get(&carts, `SELECT count(*) FROM cart_items`); err != nil {
		t.Fatal(err)
	}
	if carts != 0 {
		t.Fatal("successful checkout left cart lines")
	}
}

func TestPostgresHTTPAuditWriteFailureRedactionAndRetry(t *testing.T) {
	var logs bytes.Buffer
	oldOut, oldErr := gin.DefaultWriter, gin.DefaultErrorWriter
	gin.DefaultWriter, gin.DefaultErrorWriter = &logs, &logs
	t.Cleanup(func() { gin.DefaultWriter, gin.DefaultErrorWriter = oldOut, oldErr })
	h, db, buyer, _, _, variant := auditHTTP(t)
	var second int64
	if err := db.Get(&second, `SELECT id FROM product_variants WHERE id<>$1 AND stock>0 ORDER BY id LIMIT 1`, variant); err != nil {
		t.Fatal(err)
	}
	for _, v := range []int64{variant, second} {
		buyer.call("POST", "/cart/items", fmt.Sprintf(`{"product_variant_id":%d,"quantity":1}`, v), 200)
	}
	if _, err := db.Exec(fmt.Sprintf(`CREATE FUNCTION fail_audit_line() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.product_variant_id=%d THEN RAISE EXCEPTION 'audit_SQL_secret_493'; END IF; RETURN NEW; END $$`, second)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER fail_audit_line BEFORE INSERT ON order_items FOR EACH ROW EXECUTE FUNCTION fail_audit_line()`); err != nil {
		t.Fatal(err)
	}
	before := httpAuditState(t, db)
	request := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/orders?email=audit_private_query", strings.NewReader(`{}`))
		r.Header.Set("Authorization", "Bearer "+buyer.token)
		r.Header.Set("Idempotency-Key", "audit-aborted-key")
		w := httptest.NewRecorder()
		h.router.ServeHTTP(w, r)
		return w
	}
	w := request()
	if w.Code != 500 || strings.TrimSpace(w.Body.String()) != `{"error":"shopping operation failed"}` || before != httpAuditState(t, db) {
		t.Fatal("write failure leaked or partially committed", w.Code)
	}
	if _, err := db.Exec(`DROP TRIGGER fail_audit_line ON order_items`); err != nil {
		t.Fatal(err)
	}
	if w = request(); w.Code != 201 {
		t.Fatal("aborted request prevented retry", w.Code)
	}
	for _, secret := range []string{"audit_SQL_secret_493", "audit_private_query", buyer.token, "audit-aborted-key"} {
		if strings.Contains(logs.String(), secret) || strings.Contains(w.Body.String(), secret) {
			t.Fatal("secret leaked")
		}
	}
	var n int
	if err := db.Get(&n, `SELECT count(*) FROM order_items`); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatal("retry lost snapshots")
	}
}

func TestPostgresHTTPReservationProtectsLastUnitAndReturnsToCatalog(t *testing.T) {
	_, db, buyer, other, product, variant := auditHTTP(t)
	if _, err := db.Exec(`UPDATE product_variants SET stock=CASE WHEN id=$1 THEN 1 ELSE 0 END WHERE product_color_id IN(SELECT id FROM product_colors WHERE product_id=$2)`, variant, product); err != nil {
		t.Fatal(err)
	}
	available := func() bool {
		cards := pvmDecode[[]productCardResponse](t, buyer.call("GET", "/products", "", 200))
		for _, card := range cards {
			if card.ID == product {
				return card.IsAvailable
			}
		}
		t.Fatal("published product disappeared")
		return false
	}
	body := fmt.Sprintf(`{"product_variant_id":%d,"quantity":1}`, variant)
	if !available() {
		t.Fatal("fixture has no available unit")
	}
	buyer.call("POST", "/cart/items", body, 200)
	other.call("POST", "/cart/items", body, 200)
	order := pvmDecode[orderResponse](t, buyer.call("POST", "/orders", `{}`, 201))
	if available() {
		t.Fatal("reserved unit still available in public catalog")
	}
	before := httpAuditState(t, db)
	other.call("POST", "/orders", `{}`, 409)
	if before != httpAuditState(t, db) {
		t.Fatal("losing checkout changed state")
	}
	buyer.call("POST", fmt.Sprintf("/orders/%d/cancel", order.ID), `{}`, 200)
	if !available() {
		t.Fatal("cancelled reservation did not return to public catalog")
	}
	second := pvmDecode[orderResponse](t, other.call("POST", "/orders", `{}`, 201))
	if second.Status != "unpaid" || second.ID == order.ID || available() {
		t.Fatal("returned unit not reserved for the other buyer")
	}
	var stock int64
	if err := db.Get(&stock, `SELECT stock FROM product_variants WHERE id=$1`, variant); err != nil {
		t.Fatal(err)
	}
	if stock != 0 {
		t.Fatal(stock)
	}
}
