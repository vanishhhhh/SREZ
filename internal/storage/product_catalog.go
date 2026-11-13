package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/lib/pq"

	"srez/internal/usecase"
)

// Only these server-owned ORDER BY expressions can enter the query text.
var productSortSQL = map[string]string{
	"id_asc":     "p.id ASC",
	"price_asc":  "p.price_kopecks ASC, p.id ASC",
	"price_desc": "p.price_kopecks DESC, p.id ASC",
	"newest":     "p.created_at DESC, p.id DESC",
	"oldest":     "p.created_at ASC, p.id ASC",
}

func catalogSelection(q usecase.ProductQuery) (string, []any) {
	var conditions strings.Builder
	conditions.WriteString(publicProducts)
	args := []any{}
	add := func(expression string, value any) {
		args = append(args, value)
		conditions.WriteString(fmt.Sprintf(expression, len(args)))
	}
	if q.Category != "" {
		add(" AND p.category = $%d", q.Category)
	}
	if q.BrandID != 0 {
		add(" AND p.brand_id = $%d", q.BrandID)
	}
	if q.MinPrice != nil {
		add(" AND p.price_kopecks >= $%d", *q.MinPrice)
	}
	if q.MaxPrice != nil {
		add(" AND p.price_kopecks <= $%d", *q.MaxPrice)
	}
	if q.Size != "" || q.Color != "" {
		conditions.WriteString(` AND EXISTS (SELECT 1 FROM product_colors fc JOIN product_variants fv ON fv.product_color_id = fc.id WHERE fc.product_id = p.id AND fv.stock > 0`)
		if q.Size != "" {
			add(" AND fv.size_key = $%d", q.Size)
		}
		if q.Color != "" {
			add(" AND fc.color_key = $%d", q.Color)
		}
		conditions.WriteString(")")
	}
	return conditions.String(), args
}

// BrowsePublic uses at most three statements regardless of the page length:
// count, products with brands, and all colors/variants with their first images.
// All statements share one snapshot, including the count and stock indicators.
func (p *Products) BrowsePublic(ctx context.Context, query usecase.ProductQuery) (*usecase.ProductPage, error) {
	q, err := usecase.NormalizeProductQuery(query)
	if err != nil {
		return nil, err
	}
	tx, err := p.db.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	page := &usecase.ProductPage{Items: []usecase.ProductCard{}, Page: q.Page, Limit: q.Limit}
	selection, args := catalogSelection(q)
	if err = tx.GetContext(ctx, &page.Total, "SELECT count(*)"+selection, args...); err != nil {
		return nil, err
	}
	var rows []struct {
		productRow
		BrandName string `db:"brand_name"`
		BrandSlug string `db:"brand_slug"`
	}
	args = append(args, q.Limit, (q.Page-1)*int64(q.Limit))
	statement := "SELECT " + productColumns + ", b.name AS brand_name, b.slug AS brand_slug" + selection +
		" ORDER BY " + productSortSQL[q.Sort] + fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	if err = tx.SelectContext(ctx, &rows, statement, args...); err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	positions := make(map[int64]int, len(rows))
	for _, row := range rows {
		product, err := row.product()
		if err != nil {
			return nil, err
		}
		positions[product.ID()] = len(page.Items)
		ids = append(ids, product.ID())
		page.Items = append(page.Items, usecase.ProductCard{
			Product: *product, BrandName: row.BrandName, BrandSlug: row.BrandSlug, Colors: []usecase.ProductCardColor{},
		})
	}
	if len(ids) > 0 {
		var children []struct {
			ProductID       int64          `db:"product_id"`
			ColorID         int64          `db:"color_id"`
			Color           string         `db:"color"`
			PreviewImageURL string         `db:"preview_image_url"`
			VariantID       sql.NullInt64  `db:"variant_id"`
			Size            sql.NullString `db:"size"`
			Stock           sql.NullInt64  `db:"stock"`
		}
		err = tx.SelectContext(ctx, &children, `SELECT c.product_id, c.id AS color_id, c.color,
 COALESCE(image.url, '') AS preview_image_url, v.id AS variant_id, v.size, v.stock
 FROM product_colors c
 LEFT JOIN LATERAL (SELECT url FROM product_color_images WHERE product_color_id = c.id ORDER BY position LIMIT 1) image ON true
 LEFT JOIN product_variants v ON v.product_color_id = c.id
 WHERE c.product_id = ANY($1) ORDER BY c.product_id, c.id, v.id`, pq.Array(ids))
		if err != nil {
			return nil, err
		}
		colorPositions := map[int64]int{}
		for _, child := range children {
			card := &page.Items[positions[child.ProductID]]
			colorIndex, exists := colorPositions[child.ColorID]
			if !exists {
				colorIndex = len(card.Colors)
				colorPositions[child.ColorID] = colorIndex
				card.Colors = append(card.Colors, usecase.ProductCardColor{
					ID: child.ColorID, Color: child.Color, PreviewImageURL: child.PreviewImageURL,
					Variants: []usecase.ProductCardVariant{},
				})
				if card.PreviewImageURL == "" {
					card.PreviewImageURL = child.PreviewImageURL
				}
			}
			if child.VariantID.Valid {
				card.Colors[colorIndex].Variants = append(card.Colors[colorIndex].Variants, usecase.ProductCardVariant{
					ID: child.VariantID.Int64, Size: child.Size.String, Stock: child.Stock.Int64,
				})
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return page, nil
}
