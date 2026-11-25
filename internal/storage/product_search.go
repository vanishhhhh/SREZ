package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"

	"srez/internal/catalogsearch"
	"srez/internal/usecase"
)

func (p *Products) browseIndexed(ctx context.Context, q usecase.ProductQuery) (*usecase.ProductPage, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	tx, err := p.db.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var version struct {
		Namespace string
		Revision  int64
	}
	if err = tx.GetContext(ctx, &version, "SELECT namespace::text, revision FROM catalog_search_revision WHERE singleton"); err != nil {
		return nil, err
	}
	alias := "srez-catalog-v1-" + version.Namespace
	revision := fmt.Sprintf("%s-r%d", alias, version.Revision)
	ids, total, err := p.searchRevision(ctx, tx, alias, revision, q)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", usecase.ErrCatalogSearchUnavailable, err)
	}
	page := &usecase.ProductPage{Items: []usecase.ProductCard{}, Total: total, Page: q.Page, Limit: q.Limit}
	if len(ids) != 0 {
		var rows []productCardRow
		err = tx.SelectContext(ctx, &rows,
			"SELECT "+productColumns+", b.name AS brand_name, b.slug AS brand_slug"+publicProducts+
				" AND p.id = ANY($1) ORDER BY array_position($1::bigint[], p.id)", pq.Array(ids))
		if err != nil {
			return nil, err
		}
		if err = loadCards(ctx, tx, page, rows); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return page, nil
}

func (p *Products) searchRevision(ctx context.Context, tx *sqlx.Tx, alias, revision string, q usecase.ProductQuery) ([]int64, int64, error) {
	ids, total, err := p.index.Search(ctx, revision, q)
	if !errors.Is(err, catalogsearch.ErrMissingRevision) {
		return ids, total, err
	}
	// Serialize index publishing across API instances using the same database.
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", alias); err != nil {
		return nil, 0, err
	}
	ids, total, err = p.index.Search(ctx, revision, q)
	if !errors.Is(err, catalogsearch.ErrMissingRevision) {
		return ids, total, err
	}
	docs, err := indexableProducts(ctx, tx)
	if err != nil {
		return nil, 0, err
	}
	if err = p.index.Replace(ctx, alias, revision, docs); err != nil {
		return nil, 0, err
	}
	return p.index.Search(ctx, revision, q)
}

func indexableProducts(ctx context.Context, tx *sqlx.Tx) ([]catalogsearch.Document, error) {
	var rows []struct {
		ID        int64
		BrandID   int64 `db:"brand_id"`
		Category  string
		Price     int64
		CreatedAt time.Time `db:"created_at"`
		Variants  []byte
	}
	err := tx.SelectContext(ctx, &rows, `SELECT p.id, p.brand_id, p.category, p.price_kopecks AS price, p.created_at,
 COALESCE((SELECT jsonb_agg(jsonb_build_object('color', c.color_key, 'size', v.size_key))
 FROM product_colors c JOIN product_variants v ON v.product_color_id = c.id
 WHERE c.product_id = p.id AND v.stock > 0), '[]'::jsonb) AS variants`+publicProducts)
	if err != nil {
		return nil, err
	}
	docs := make([]catalogsearch.Document, 0, len(rows))
	for _, row := range rows {
		doc := catalogsearch.Document{
			ID: row.ID, BrandID: row.BrandID, Category: row.Category, Price: row.Price, CreatedAt: row.CreatedAt.UnixMicro(),
		}
		if err = json.Unmarshal(row.Variants, &doc.Variants); err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}
	return docs, nil
}
