package storage_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"net/url"
	"os"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"

	"srez/internal/storage"
	"srez/internal/usecase"
)

func TestPostgresCatalogFiltersAndCards(t *testing.T) {
	db := testDB(t)
	migrate(t, db)
	seed(t, db)
	ctx := context.Background()
	repo := storage.NewProducts(db)
	initial, err := repo.BrowsePublic(ctx, usecase.ProductQuery{})
	if err != nil || initial.Total != 2 || len(initial.Items) != 2 {
		t.Fatalf("initial page: %#v, %v", initial, err)
	}
	first, second := initial.Items[0], initial.Items[1]
	if first.BrandName != "Срез Студия" || first.BrandSlug != "srez-studio" || first.PreviewImageURL != "https://example.com/products/1/black.png" ||
		len(first.Colors) != 2 || len(first.Colors[0].Variants) != 2 || first.Colors[0].Variants[1].Stock != 0 {
		t.Fatalf("missing card data: %#v", first)
	}
	price := int64(250000)
	for _, tt := range []struct {
		name string
		q    usecase.ProductQuery
		ids  []int64
	}{
		{"category", usecase.ProductQuery{Category: "hoodie"}, []int64{second.Product.ID()}},
		{"brand", usecase.ProductQuery{BrandID: first.Product.BrandID()}, []int64{first.Product.ID()}},
		{"price inclusive", usecase.ProductQuery{MinPrice: &price, MaxPrice: &price}, []int64{first.Product.ID()}},
		{"minimum", usecase.ProductQuery{MinPrice: &price}, []int64{first.Product.ID(), second.Product.ID()}},
		{"stocked size", usecase.ProductQuery{Size: " M "}, []int64{first.Product.ID(), second.Product.ID()}},
		{"sold out size", usecase.ProductQuery{Size: "L"}, []int64{}},
		{"matching color size", usecase.ProductQuery{Color: " БЕЛЫЙ ", Size: "one size"}, []int64{first.Product.ID(), second.Product.ID()}},
		{"different color size", usecase.ProductQuery{Color: "Чёрный", Size: "One Size"}, []int64{}},
		{"combined", usecase.ProductQuery{Category: "tshirt", BrandID: first.Product.BrandID(), Size: "M", MaxPrice: &price}, []int64{first.Product.ID()}},
		{"missing brand", usecase.ProductQuery{BrandID: 9223372036854775807}, []int64{}},
		{"SQL data is not code", usecase.ProductQuery{Color: "' OR true --"}, []int64{}},
		{"price descending", usecase.ProductQuery{Sort: "price_desc"}, []int64{second.Product.ID(), first.Product.ID()}},
		{"price ascending", usecase.ProductQuery{Sort: "price_asc"}, []int64{first.Product.ID(), second.Product.ID()}},
		{"newest tie", usecase.ProductQuery{Sort: "newest"}, []int64{second.Product.ID(), first.Product.ID()}},
		{"oldest tie", usecase.ProductQuery{Sort: "oldest"}, []int64{first.Product.ID(), second.Product.ID()}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			page, err := repo.BrowsePublic(ctx, tt.q)
			if err != nil {
				t.Fatal(err)
			}
			ids := []int64{}
			for _, item := range page.Items {
				ids = append(ids, item.Product.ID())
			}
			if !reflect.DeepEqual(ids, tt.ids) || page.Total != int64(len(tt.ids)) {
				t.Fatalf("ids=%v total=%d, want %v", ids, page.Total, tt.ids)
			}
		})
	}
	for _, tt := range []struct {
		page int64
		ids  []int64
	}{{1, []int64{first.Product.ID()}}, {2, []int64{second.Product.ID()}}, {3, []int64{}}, {9223372036854775807, []int64{}}} {
		page, err := repo.BrowsePublic(ctx, usecase.ProductQuery{Page: tt.page, Limit: 1})
		if err != nil || page.Total != 2 || page.Page != tt.page || page.Limit != 1 {
			t.Fatalf("pagination: %#v %v", page, err)
		}
		ids := []int64{}
		for _, item := range page.Items {
			ids = append(ids, item.Product.ID())
		}
		if !reflect.DeepEqual(ids, tt.ids) {
			t.Fatalf("page %d: %v", tt.page, ids)
		}
	}
	// Repeated prices must not repeat/skip records between pages.
	exec(t, db, `UPDATE products SET price_kopecks=250000`)
	page, err := repo.BrowsePublic(ctx, usecase.ProductQuery{Sort: "price_desc", Page: 2, Limit: 1})
	if err != nil || len(page.Items) != 1 || page.Items[0].Product.ID() != second.Product.ID() {
		t.Fatalf("unstable price tie ordering: %#v %v", page, err)
	}
	for _, status := range []string{"draft", "hidden"} {
		exec(t, db, `UPDATE products SET status=$2 WHERE id=$1`, first.Product.ID(), status)
		page, err := repo.BrowsePublic(ctx, usecase.ProductQuery{})
		if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].Product.ID() != second.Product.ID() {
			t.Fatalf("private product exposed: %#v %v", page, err)
		}
	}
	exec(t, db, `UPDATE products SET status='published' WHERE id=$1`, first.Product.ID())
	for _, status := range []string{"draft", "suspended"} {
		exec(t, db, `UPDATE brands SET status=$2 WHERE id=$1`, first.Product.BrandID(), status)
		page, err := repo.BrowsePublic(ctx, usecase.ProductQuery{BrandID: first.Product.BrandID()})
		if err != nil || page.Total != 0 || len(page.Items) != 0 {
			t.Fatalf("private brand exposed: %#v %v", page, err)
		}
	}
}

// Instrument the real PostgreSQL driver to guard the no-N+1 requirement.
type countedConnector struct {
	driver.Connector
	queries *atomic.Int64
}

func (c countedConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.Connector.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return countedConn{Conn: conn, queries: c.queries}, nil
}

type countedConn struct {
	driver.Conn
	queries *atomic.Int64
}

func (c countedConn) BeginTx(ctx context.Context, options driver.TxOptions) (driver.Tx, error) {
	return c.Conn.(driver.ConnBeginTx).BeginTx(ctx, options)
}

func (c countedConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.queries.Add(1)
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
}

func TestPostgresCatalogUsesFixedQueryCount(t *testing.T) {
	db := testDB(t)
	migrate(t, db)
	seed(t, db)
	var schema string
	if err := db.Get(&schema, `SELECT current_schema()`); err != nil {
		t.Fatal(err)
	}
	dsn, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	params := dsn.Query()
	params.Set("search_path", schema)
	dsn.RawQuery = params.Encode()
	connector, err := pq.NewConnector(dsn.String())
	if err != nil {
		t.Fatal(err)
	}
	var queries atomic.Int64
	countedDB := sqlx.NewDb(sql.OpenDB(countedConnector{Connector: connector, queries: &queries}), "postgres")
	t.Cleanup(func() { countedDB.Close() })
	repo := storage.NewProducts(countedDB)
	for _, limit := range []int{1, 100} {
		queries.Store(0)
		page, err := repo.BrowsePublic(context.Background(), usecase.ProductQuery{Limit: limit})
		if err != nil || page.Total != 2 {
			t.Fatalf("page: %#v %v", page, err)
		}
		if got := queries.Load(); got != 3 {
			t.Fatalf("limit=%d: got %d queries, want 3", limit, got)
		}
	}
}
