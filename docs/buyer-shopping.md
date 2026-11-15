# Buyer wishlist, cart, checkout and order history

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
No existing wishlist/cart/order contract was found, so the defaults below are explicit.

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
| POST | `/orders` | `{}` | 201, one unpaid order for the whole cart |
| GET | `/orders` | none | 200, own orders newest first |
| GET | `/orders/:id` | none | 200, owned order; foreign/absent is 404 |

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
a management/cart lock inversion. Order snapshots deliberately have no live catalog FKs.

## Checkout and history

`POST /orders` requires exactly `{}` as its JSON body. No owner, items, delivery,
status, total, price or payment data is accepted. It purchases **all** current
cart lines, including different brands, as one order. There is no partial purchase:
a single unavailable or insufficient line rejects the checkout with 409. Prices
are re-read on checkout and may differ from a previous GET /cart. New orders are
always `unpaid`; no payment or fulfillment transitions are exposed.

All three order endpoints use the same DTO. POST returns one object (201),
GET /orders/:id one object (200), GET /orders an array (200, `[]` if empty):

```json
{
  "id":41,
  "status":"unpaid",
  "total_kopecks":600000,
  "created_at":"2026-11-15T12:00:00Z",
  "items":[
    {"product_variant_id":21,"product_id":7,"brand_id":2,"product_name":"Hoodie","color":"Black","size":"M","quantity":2,"unit_price_kopecks":250000,"subtotal_kopecks":500000},
    {"product_variant_id":22,"product_id":9,"brand_id":3,"product_name":"Tee","color":"White","size":"L","quantity":1,"unit_price_kopecks":100000,"subtotal_kopecks":100000}
  ]
}
```

Order list sorts by ID descending, lines by insertion ID (cart item order at
checkout). The minimal history API is unpaginated; pagination is follow-up work.
Dates use RFC 3339. DTOs omit user_id, and order queries always scope by the JWT
buyer, including line reads joined through orders.user_id. Foreign IDs return
404 without disclosing their existence. No separate per-order line query is
issued: list and detail load headers and all owned lines in two batch queries
inside one read-only REPEATABLE READ transaction.

Migration **007_buyer_orders.sql** adds orders and order_items with buyer/order
FKs, quantity/money/status checks, one variant per order, and buyer/order indexes.
All catalog IDs (brand, product, variant) in order_items are immutable historical
values without live catalog FKs. Text, color, size, quantity and unit price are
copied; name, price and catalog publication/deletion never affect the order.
The domain Order has no setters, copies its snapshots, and validates the stored
sum on restore. The API cannot alter snapshots or client-select a status.
Runtime SQL access must remain restricted as with existing catalog invariants.
No earlier migration is edited or renumbered; deployment uses the same runner.

### Checkout atomicity and locking

WithinCheckout reuses the same negative-buyer advisory lock as cart writes,
including empty carts. This serializes simultaneous checkouts of one cart and
cart add/PATCH/delete against checkout. It discovers brand IDs from the owned
cart inside the transaction, then acquires **all brand rows in ascending ID
order** before locking products (ID order), colors (ID order), images
(color/position order), and variants (ID order). It follows the existing catalog
hierarchy; buyer operations do not need management memberships. Management locks
only one brand at a time, so sorting all brands makes cross-brand checkout
compatible with its protocol. Opposite cart insertion order cannot invert locks.
No cart row lock is acquired before catalog locks.

Only after the locks are held does checkout re-read the actual cart and current
public products/active brands, prices, colors, sizes and stock. A variant deleted
while checkout waited appears as an unavailable tombstone; price/status/stock
changes committed before acquiring the brand lock are observed. Supported catalog
writers share these brand locks; direct SQL must respect the protocol.

Usecase rejects empty carts, unavailable lines, insufficient stock, and money
overflow, then creates an immutable Order through entities. Storage inserts the
header and snapshots, conditionally decrements stock with `stock >= quantity`
and an ownership-scoped cart match, and deletes only the corresponding buyer's
purchased cart lines. All writes share one database transaction. A write failure
rolls back even already-processed lines, order rows, inventory and cart deletion.
PostgreSQL stock CHECK is a further defense against negative stock. Cart-only
operations do not reserve inventory; buyers may both cart the last unit, but
exactly one checkout can buy it. The losing buyer receives 409 and keeps the cart.

**Repeated request behavior:** after successful checkout the cart is empty, so
an immediate retry returns `409 {"error":"cart is empty"}` and creates no second
order. Concurrent same-cart requests yield one 201 and one empty-cart 409.
There is no Idempotency-Key/response replay: after a lost success response, read
GET /orders. If new items were added before retry, it is a new checkout of that
current cart. Failed checkouts do not clear the cart and can be retried after the
buyer fixes the problem. Orders retain their unpaid status and decremented stock
until a later fulfillment/payment/cancellation design is implemented.

## Follow-up decisions

Guest cart persistence/merge, payment simulation, brand-side fulfillment, partial
cancellation and delivery remain outside these milestones. The requested v0.3
was unavailable, so its exact open decisions cannot be quoted or verified.
Before those features, resolve against that specification:

- Guest merge identity, duplicate-variant quantities, stock conflicts and limits.
- Payment simulation contract, idempotency/retries, unpaid expiry and whether/how
  stock is restored on failure or cancellation; no automatic restore exists now.
- Brand-scoped fulfillment permissions and line/suborder statuses while preserving
  the buyer's one multi-brand order and immutable snapshots.
- Partial cancellation eligibility, refund/accounting rules and inventory returns.
- Delivery addresses/contact data, retention/access, shipping fees and per-brand
  shipment grouping, including their effect on order totals and snapshots.

These are pending implementation questions, not claims about unseen v0.3 text.

## Errors

| Status | Reason |
| --- | --- |
| 400 | Invalid ID/JSON, unsupported fields or quantity outside 1–100 |
| 401 | Missing/invalid Bearer JWT |
| 404 | Unknown/non-public variant or product on add; absent/foreign cart item or order |
| 409 | Empty checkout, unavailable checkout/PATCH line, insufficient stock, money overflow |
| 413 | Body exceeds 64 KiB |
| 500 | Unexpected persistence failure (generic message) |
| 503 | Shopping/orders dependency not configured; normal startup configures both |

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

### Checkout verification in PowerShell

After the wishlist/cart example, add a line again and check out. To exercise a
multi-brand order, add a public variant from a second brand before POST /orders.

```powershell
$body = @{ product_variant_id = $variant.id; quantity = 1 } | ConvertTo-Json
Invoke-RestMethod -Method Post -Uri "$api/cart/items" -Headers $ownerHeaders `
    -ContentType 'application/json' -Body $body | Out-Null
$order = Invoke-RestMethod -Method Post -Uri "$api/orders" -Headers $ownerHeaders `
    -ContentType 'application/json' -Body '{}'
$order | ConvertTo-Json -Depth 10 # unpaid; one immutable snapshot per line
Invoke-RestMethod -Uri "$api/cart" -Headers $ownerHeaders # empty
Invoke-RestMethod -Uri "$api/orders" -Headers $ownerHeaders | ConvertTo-Json -Depth 10
Invoke-RestMethod -Uri "$api/orders/$($order.id)" -Headers $ownerHeaders
# Expect 409 from an immediate second POST /orders, not another order.
try {
    Invoke-RestMethod -Method Post -Uri "$api/orders" -Headers $ownerHeaders `
        -ContentType 'application/json' -Body '{}'
} catch { $_.Exception.Response.StatusCode }
```

Register/login a different buyer and GET the same order ID with their JWT (404).
Change the original product price via PVM/management; GET the existing order and
confirm its price/name/color/size snapshot is unchanged. Only modify local demo
fixtures. Automated PostgreSQL tests also cover simultaneous purchase of the last
unit, multi-brand requests in opposite cart order, concurrent checkout of one
cart, locked price re-read, and injected write failure after a preceding line was
already written/decremented/deleted (full rollback).

## Verification performed

Verified on 15 November 2026 with Go 1.27.2 and a newly created, dedicated
PostgreSQL 16 test container/database (buyer_test, port 15432):

- `gofmt` applied; `gofmt -l cmd internal migrations` returned no files.
- `go test ./... -count=1 -timeout 180s` passed with TEST_DATABASE_URL set;
  PostgreSQL integration tests executed, with no database tests skipped.
- `go vet ./...` and `git diff --check` passed.
- `go test -race ./internal/storage ./internal/handler -run 'Shopping|Orders|Checkout' -count=1 -timeout 180s`
  passed on the same dedicated test database.
- Milestone 1 was independently tested before checkout work. The added suite
  contains 16 test functions plus subcases covering both milestones.

An initial optional race build hit the workspace disk limit; after removing an
unused toolchain container image downloaded for this task, its final run passed.
The PowerShell commands are documented but were not executed here (pwsh is not
installed). Team specification v0.3 verification remains unavailable as described
above. No development or production database was used.
