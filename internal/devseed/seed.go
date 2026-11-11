package devseed

import (
	"context"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"srez/internal/entities"
)

// Apply inserts each demo brand and its aggregate only when the slug is absent.
// Existing brands (including unrelated records with these slugs) are untouched.
// A transaction and advisory lock make repeated/concurrent invocations safe.
func Apply(ctx context.Context, db *sqlx.DB) error {
	brands, err := Brands()
	if err != nil {
		return err
	}
	products, err := Products()
	if err != nil {
		return err
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(738291402)`); err != nil {
		return err
	}
	for _, b := range brands {
		var exists bool
		if err = tx.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM brands WHERE slug=$1)`, b.Slug()); err != nil {
			return err
		}
		if exists {
			continue
		}
		var brandID int64
		c, a := b.Contacts(), b.Appearance()
		if err = tx.GetContext(ctx, &brandID, `INSERT INTO brands(name,slug,description,logo_url,banner_url,contact_email,telegram_url,website_url,accent_color,background_color,text_color,status,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING id`, b.Name(), b.Slug(), b.Description(), b.LogoURL(), b.BannerURL(), c.Email, c.TelegramURL, c.WebsiteURL, a.AccentColor, a.BackgroundColor, a.TextColor, b.Status(), b.CreatedAt(), b.UpdatedAt()); err != nil {
			return err
		}
		for _, entry := range products {
			p := entry.Product
			if p.BrandID() != b.ID() {
				continue
			}
			var productID int64
			if err = tx.GetContext(ctx, &productID, `INSERT INTO products(brand_id,name,description,price_kopecks,category,materials,status,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,'draft',$7,$8) RETURNING id`, brandID, p.Name(), p.Description(), p.PriceKopecks(), p.Category(), pq.Array(p.Materials()), p.CreatedAt(), p.UpdatedAt()); err != nil {
				return err
			}
			state := p.State()
			state.ID = productID
			state.BrandID = brandID
			state.Status = entities.ProductDraft
			saved, err := entities.RestoreProduct(state)
			if err != nil {
				return err
			}
			publication := entities.ProductPublication{}
			for _, c := range entry.Publication.Colors {
				var colorID int64
				if err = tx.GetContext(ctx, &colorID, `INSERT INTO product_colors(product_id,color,color_key) VALUES($1,$2,$3) RETURNING id`, productID, c.Color(), key(c.Color())); err != nil {
					return err
				}
				color, err := entities.RestoreProductColor(entities.ProductColorState{ID: colorID, ProductID: productID, Color: c.Color(), Images: c.Images()})
				if err != nil {
					return err
				}
				publication.Colors = append(publication.Colors, *color)
				for position, url := range c.Images() {
					if _, err = tx.ExecContext(ctx, `INSERT INTO product_color_images(product_color_id,position,url) VALUES($1,$2,$3)`, colorID, position, url); err != nil {
						return err
					}
				}
				for _, v := range entry.Publication.Variants {
					if v.ProductColorID() != c.ID() {
						continue
					}
					var variantID int64
					if err = tx.GetContext(ctx, &variantID, `INSERT INTO product_variants(product_color_id,size,size_key,stock,sku) VALUES($1,$2,$3,$4,$5) RETURNING id`, colorID, v.Size(), key(v.Size()), v.Stock(), v.SKU()); err != nil {
						return err
					}
					variant, err := entities.RestoreProductVariant(entities.ProductVariantState{ID: variantID, ProductColorID: colorID, Size: v.Size(), Stock: v.Stock(), SKU: v.SKU()})
					if err != nil {
						return err
					}
					publication.Variants = append(publication.Variants, *variant)
				}
			}
			if err = saved.Publish(publication, p.UpdatedAt()); err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, `UPDATE products SET status=$2 WHERE id=$1`, productID, saved.Status()); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
func key(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
