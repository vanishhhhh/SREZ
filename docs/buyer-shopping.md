# Buyer wishlist and cart

All routes require `Authorization: Bearer <access_token>` and return
`Cache-Control: no-store`, including errors. The existing JWT middleware supplies
the buyer ID; cookie-only access is rejected. Owner IDs, independent color/size,
brand, prices and totals are never accepted from clients. Auth, profile, catalog
and brand-management contracts are unchanged. Errors use `{"error":"message"}`
without SQL or account data; existing safe access logging/recovery applies.

## Sources and decisions

The implementation follows `architecture.md` and the supplied buyer milestones.
The requested team specification v0.3 was absent from the attachments and the
current public repository (base `52d6f39`); only `docs/legacy/SPEC.md` was present.
That historical document is not a product contract. Validation against v0.3,
including its open decisions, remains pending availability of that specification.
No existing wishlist/cart contract was found, so the defaults below are explicit.

## Endpoints

| Method | Path | Request | Success |
| --- | --- | --- | --- |
| GET | `/favorites` | none | 200, array of saved entries |
| POST | `/favorites/:product_id` | none | 204, idempotent add |
| DELETE | `/favorites/:product_id` | none | 204, idempotent remove |
| GET | `/cart` | none | 200, cart object |
| POST | `/cart/items` | `{"product_variant_id":21,"quantity":2}` | 200, resulting cart; increments existing quantity |
| PATCH | `/cart/items/:id` | `{"quantity":3}` | 200, resulting cart; sets absolute quantity |
| DELETE | `/cart/items/:id` | none | 204; unknown/another buyer's line is 404 |

IDs are positive int64. Quantities must be JSON integers from **1 to 100**, both
for the request and resulting line; PATCH requires quantity. Missing/null, zero,
negative, fractional, string and overflowing values fail with 400. Use DELETE
instead of quantity=0. Body endpoints use the existing strict JSON decoder:
exactly one object, up to 64 KiB, unknown fields rejected. Favorites operations
are bodyless. POST cart success is always 200 for either insert or increment.

### Favorites

```json
[
  {"product_id":7,"product":{"id":7,"brand_id":2,"name":"Hoodie","description":"Cotton hoodie","price_kopecks":250000,"category":"hoodie","materials":["cotton"],"status":"published","created_at":"2026-11-15T10:00:00Z","updated_at":"2026-11-15T10:00:00Z","preview_image_url":"https://example.com/front.jpg","brand_name":"Example","brand_slug":"example","is_available":true,"available_colors":["Black"],"available_sizes":["M"],"colors":[{"id":11,"color":"Black","preview_image_url":"https://example.com/front.jpg","is_available":true,"available_sizes":["M"]}]}},
  {"product_id":8,"product":null}
]
```

`product` reuses the public catalog card DTO. Entries are ordered by product ID.
Products must be published and their brands active to be added; zero stock is
allowed. Duplicate adds use atomic `ON CONFLICT DO NOTHING`. A product that later
becomes hidden/draft or whose brand is inactive has `product:null`; its saved ID
remains removable. Re-adding an unavailable product returns 404, even if already
saved. Removing absent entries always returns 204 and touches only this buyer.
An empty list is `[]`. Loading uses at most three queries in one read-only
REPEATABLE READ snapshot (relationships, public products, all children), sharing
the public catalog batch projection; no per-product requests.

### Cart

```json
{
  "items":[
    {"id":31,"product_variant_id":21,"quantity":2,"product":{"product_id":7,"brand_id":2,"name":"Hoodie","preview_image_url":"https://example.com/front.jpg","color":"Black","size":"M","unit_price_kopecks":250000},"availability":"available","subtotal_kopecks":500000},
    {"id":32,"product_variant_id":22,"quantity":3,"product":{"product_id":9,"brand_id":3,"name":"Tee","preview_image_url":"https://example.com/tee.jpg","color":"White","size":"L","unit_price_kopecks":100000},"availability":"insufficient_stock","subtotal_kopecks":0},
    {"id":33,"product_variant_id":null,"quantity":1,"product":null,"availability":"unavailable","subtotal_kopecks":0}
  ],
  "total_kopecks":500000
}
```

One cart per user is implicit in `cart_items.user_id`. A cart may contain several
brands. One line per live variant is enforced by UNIQUE(user_id,product_variant_id).
Items are ordered by cart item ID. Preview is the first image of the selected
color, by position. Prices are current catalog prices in integer kopecks.
Availability is `available`, `insufficient_stock` (public but stock < quantity),
or `unavailable` (deleted variant or non-public catalog). Only **available** lines
contribute to totals; all other subtotals are zero. Insufficient lines retain
public product information; unavailable lines expose no catalog fields, with
`product:null`. A hidden variant's previously saved ID remains; a deleted variant
has `product_variant_id:null`. Empty cart: `{"items":[],"total_kopecks":0}`.
Checked multiplication/addition rejects money exceeding int64 with 409. Cart
mutations also calculate the resulting cart before commit, rolling back on overflow.
A later price change can make GET fail with 409; DELETE remains available.

POST requires a currently public variant and resulting quantity <= stock.
PATCH cannot change unavailable lines (409); they remain removable. A public
line with insufficient stock may be reduced to a quantity that fits. Every
cart item lookup/update/delete includes user_id in SQL, so another buyer's ID
returns the same 404 as an unknown ID. Cart operations never change or reserve stock.

## Transactions and deletion

Migration **006_buyer_shopping.sql** adds favorites, cart_items, FKs, uniqueness,
quantity constraints and indexes; 001–005 are unchanged. The embedded migration
runner applies it transactionally and checks checksums as before. No downgrade
or data recreation is used.

Every cart mutation is one storage transaction serialized by a transaction
advisory lock keyed by the negative user ID. This namespace is disjoint from the
positive migration/seed keys. It also works for empty carts. Management does not
acquire this lock. POST/PATCH then lock the brand, product and selected children,
re-read publication/brand/stock, validate the resulting quantity in usecase, and
mutate. No cart row is locked before catalog locks. Concurrent adds cannot lose
increments; PostgreSQL uniqueness is an additional safeguard. GET uses one SQL
statement snapshot with SQL-level redaction and server-side totals.

PVM keeps its brand → membership → product → colors/images/variants protocol.
Variant/color deletion explicitly sets affected `cart_items.product_variant_id`
to NULL **before** deleting variants in that same management transaction. Lines
remain as removable tombstones with their ID and quantity. FK defaults forbid
accidental raw deletion while a reference exists: there is no cascading cart
removal or implicit FK detachment. Publication validation failures roll back
both deletion and detachment. Brand locks serialize adds with deletion; cart
transactions never hold cart row locks while waiting for a catalog lock, avoiding
a management/cart lock inversion. Historical order references will not use a
restrictive variant FK.

## Errors

| Status | Reason |
| --- | --- |
| 400 | Invalid ID/JSON, unsupported fields or quantity outside 1–100 |
| 401 | Missing/invalid Bearer JWT |
| 404 | Unknown/non-public variant or product on add; absent/foreign cart item |
| 409 | Insufficient stock, unavailable line on PATCH, money overflow |
| 413 | Body exceeds 64 KiB |
| 500 | Unexpected persistence failure (generic message) |
| 503 | Shopping dependency not configured; normal startup configures it |

## Local PowerShell verification

Start the app using [PVM setup](product-variant-management.md#ручная-проверка-в-windows-powershell),
including its registration/login and published product flow. It provides `$api`,
`$ownerHeaders`, `$product`, `$variant` and a variant with sufficient stock.
Refresh login if the 15-minute JWT expires. Bearer shopping calls do not use
refresh cookies or Origin; existing auth Origin/CSRF requirements remain intact.

```powershell
Invoke-RestMethod -Method Post -Uri "$api/favorites/$($product.id)" -Headers $ownerHeaders
Invoke-RestMethod -Method Post -Uri "$api/favorites/$($product.id)" -Headers $ownerHeaders
Invoke-RestMethod -Uri "$api/favorites" -Headers $ownerHeaders | ConvertTo-Json -Depth 12
$body = @{ product_variant_id = $variant.id; quantity = 1 } | ConvertTo-Json
$cart = Invoke-RestMethod -Method Post -Uri "$api/cart/items" -Headers $ownerHeaders `
    -ContentType 'application/json' -Body $body
$cart = Invoke-RestMethod -Method Post -Uri "$api/cart/items" -Headers $ownerHeaders `
    -ContentType 'application/json' -Body $body
$cart | ConvertTo-Json -Depth 8 # one line with quantity 2
$itemId = $cart.items[0].id
Invoke-RestMethod -Method Patch -Uri "$api/cart/items/$itemId" -Headers $ownerHeaders `
    -ContentType 'application/json' -Body '{"quantity":1}'
Invoke-RestMethod -Method Delete -Uri "$api/cart/items/$itemId" -Headers $ownerHeaders
Invoke-RestMethod -Method Delete -Uri "$api/favorites/$($product.id)" -Headers $ownerHeaders
```

Try calls without Authorization (401), quantity=0/101 (400), and another logged-in
user's cart item ID (404). Inspect the management variant before/after cart calls:
stock must be unchanged. Hide the product and verify `product:null`, then remove
its saved entry and cart item. Use only local fixtures you created.

### Dedicated PostgreSQL integration tests

Never use development/production DATABASE_URL. Tests use only TEST_DATABASE_URL,
create isolated schemas, then drop only their own schemas. Example independent
container (ensure the name and port are free):

```powershell
docker run --rm -d --name srez-buyer-tests -e POSTGRES_USER=buyer_test `
    -e POSTGRES_PASSWORD=buyer_test_local -e POSTGRES_DB=buyer_test `
    -p 127.0.0.1:15432:5432 postgres:16-alpine
docker exec srez-buyer-tests pg_isready -h 127.0.0.1 -U buyer_test -d buyer_test
# Continue only when accepting connections.
$env:TEST_DATABASE_URL = 'postgres://buyer_test:buyer_test_local@127.0.0.1:15432/buyer_test?sslmode=disable'
go test ./... -count=1 -timeout 180s
go vet ./...
git diff --check
docker stop srez-buyer-tests
Remove-Item Env:TEST_DATABASE_URL
```
