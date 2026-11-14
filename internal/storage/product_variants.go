package storage

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/lib/pq"
	"srez/internal/entities"
	"srez/internal/usecase"
)

var _ usecase.ManagementTx = (*managementTx)(nil)

func changedRow(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return usecase.ErrResourceHidden
	}
	return nil
}

func (t *managementTx) InsertColor(ctx context.Context, color entities.ProductColor) (*entities.ProductColor, error) {
	var id int64
	err := t.tx.GetContext(ctx, &id, `INSERT INTO product_colors(product_id,color,color_key)
 SELECT id,$2,$3 FROM products WHERE id=$1 AND brand_id=$4 RETURNING id`,
		color.ProductID(), color.Color(), strings.ToLower(strings.TrimSpace(color.Color())), t.brandID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, usecase.ErrResourceHidden
	}
	if err != nil {
		return nil, err
	}
	s := color.State()
	s.ID = id
	return entities.RestoreProductColor(s)
}
func (t *managementTx) SaveColor(ctx context.Context, color entities.ProductColor) error {
	return changedRow(t.tx.ExecContext(ctx, `UPDATE product_colors c SET color=$3,color_key=$4
 WHERE c.id=$1 AND c.product_id=$2 AND EXISTS(SELECT 1 FROM products p WHERE p.id=c.product_id AND p.brand_id=$5)`,
		color.ID(), color.ProductID(), color.Color(), strings.ToLower(strings.TrimSpace(color.Color())), t.brandID))
}
func (t *managementTx) DeleteColor(ctx context.Context, productID, colorID int64) error {
	var id int64
	err := t.tx.GetContext(ctx, &id, `SELECT c.id FROM product_colors c JOIN products p ON p.id=c.product_id
 WHERE c.id=$1 AND c.product_id=$2 AND p.brand_id=$3`, colorID, productID, t.brandID)
	if errors.Is(err, sql.ErrNoRows) {
		return usecase.ErrResourceHidden
	}
	if err != nil {
		return err
	}
	// Explicit child deletion keeps the existing no-cascade FK policy. The
	// enclosing transaction restores all children if publication validation fails.
	if _, err = t.tx.ExecContext(ctx, `DELETE FROM product_variants WHERE product_color_id=$1`, id); err != nil {
		return err
	}
	if _, err = t.tx.ExecContext(ctx, `DELETE FROM product_color_images WHERE product_color_id=$1`, id); err != nil {
		return err
	}
	return changedRow(t.tx.ExecContext(ctx, `DELETE FROM product_colors WHERE id=$1 AND product_id=$2`, id, productID))
}

func (t *managementTx) InsertVariant(ctx context.Context, v entities.ProductVariant) (*entities.ProductVariant, error) {
	var id int64
	err := t.tx.GetContext(ctx, &id, `INSERT INTO product_variants(product_color_id,size,size_key,stock,sku)
 SELECT c.id,$2,$3,$4,$5 FROM product_colors c JOIN products p ON p.id=c.product_id
 WHERE c.id=$1 AND p.brand_id=$6 RETURNING id`, v.ProductColorID(), v.Size(),
		strings.ToLower(strings.TrimSpace(v.Size())), v.Stock(), v.SKU(), t.brandID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, usecase.ErrResourceHidden
	}
	if err != nil {
		return nil, err
	}
	s := v.State()
	s.ID = id
	return entities.RestoreProductVariant(s)
}
func (t *managementTx) SaveVariant(ctx context.Context, v entities.ProductVariant) error {
	return changedRow(t.tx.ExecContext(ctx, `UPDATE product_variants v SET size=$3,size_key=$4,stock=$5,sku=$6
 WHERE v.id=$1 AND v.product_color_id=$2 AND EXISTS(SELECT 1 FROM product_colors c JOIN products p ON p.id=c.product_id
 WHERE c.id=v.product_color_id AND p.brand_id=$7)`, v.ID(), v.ProductColorID(), v.Size(),
		strings.ToLower(strings.TrimSpace(v.Size())), v.Stock(), v.SKU(), t.brandID))
}
func (t *managementTx) DeleteVariant(ctx context.Context, colorID, variantID int64) error {
	return changedRow(t.tx.ExecContext(ctx, `DELETE FROM product_variants v WHERE v.id=$1 AND v.product_color_id=$2
 AND EXISTS(SELECT 1 FROM product_colors c JOIN products p ON p.id=c.product_id WHERE c.id=v.product_color_id AND p.brand_id=$3)`, variantID, colorID, t.brandID))
}

type productImageRow struct {
	ID       int64  `db:"id"`
	ColorID  int64  `db:"product_color_id"`
	URL      string `db:"url"`
	Position int    `db:"position"`
}

func (r productImageRow) image() (*entities.ProductImage, error) {
	return entities.RestoreProductImage(entities.ProductImageState{ID: r.ID, ProductColorID: r.ColorID, URL: r.URL, Position: r.Position})
}
func (t *managementTx) Images(ctx context.Context, colorID int64) ([]entities.ProductImage, error) {
	var rows []productImageRow
	err := t.tx.SelectContext(ctx, &rows, `SELECT i.id,i.product_color_id,i.url,i.position FROM product_color_images i
 JOIN product_colors c ON c.id=i.product_color_id JOIN products p ON p.id=c.product_id
 WHERE c.id=$1 AND p.brand_id=$2 ORDER BY i.position,i.id FOR UPDATE OF i`, colorID, t.brandID)
	if err != nil {
		return nil, err
	}
	images := make([]entities.ProductImage, 0, len(rows))
	for _, r := range rows {
		image, err := r.image()
		if err != nil {
			return nil, err
		}
		images = append(images, *image)
	}
	return images, nil
}
func (t *managementTx) deferImagePositions(ctx context.Context) error {
	_, err := t.tx.ExecContext(ctx, `SET CONSTRAINTS product_color_images_pkey DEFERRED`)
	return err
}
func (t *managementTx) InsertImage(ctx context.Context, image entities.ProductImage) (*entities.ProductImage, error) {
	if err := t.deferImagePositions(ctx); err != nil {
		return nil, err
	}
	var row productImageRow
	err := t.tx.GetContext(ctx, &row, `INSERT INTO product_color_images(product_color_id,url,position)
 SELECT c.id,$2,$3 FROM product_colors c JOIN products p ON p.id=c.product_id WHERE c.id=$1 AND p.brand_id=$4
 RETURNING id,product_color_id,url,position`, image.ProductColorID(), image.URL(), image.Position(), t.brandID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, usecase.ErrResourceHidden
	}
	if err != nil {
		return nil, err
	}
	return row.image()
}
func (t *managementTx) SaveImage(ctx context.Context, image entities.ProductImage) error {
	if err := t.deferImagePositions(ctx); err != nil {
		return err
	}
	return changedRow(t.tx.ExecContext(ctx, `UPDATE product_color_images i SET url=$3,position=$4 WHERE i.id=$1 AND i.product_color_id=$2
 AND EXISTS(SELECT 1 FROM product_colors c JOIN products p ON p.id=c.product_id WHERE c.id=i.product_color_id AND p.brand_id=$5)`,
		image.ID(), image.ProductColorID(), image.URL(), image.Position(), t.brandID))
}
func (t *managementTx) DeleteImage(ctx context.Context, colorID, imageID int64) error {
	return changedRow(t.tx.ExecContext(ctx, `DELETE FROM product_color_images i WHERE i.id=$1 AND i.product_color_id=$2
 AND EXISTS(SELECT 1 FROM product_colors c JOIN products p ON p.id=c.product_id WHERE c.id=i.product_color_id AND p.brand_id=$3)`, imageID, colorID, t.brandID))
}
func (t *managementTx) SaveImagePositions(ctx context.Context, colorID int64, ids []int64) error {
	if len(ids) > 10 {
		return entities.ErrInvalidValue
	}
	var count int
	err := t.tx.GetContext(ctx, &count, `SELECT count(*) FROM product_color_images i JOIN product_colors c ON c.id=i.product_color_id
 JOIN products p ON p.id=c.product_id WHERE c.id=$1 AND p.brand_id=$2`, colorID, t.brandID)
	if err != nil {
		return err
	}
	if count != len(ids) {
		return usecase.ErrConflict
	}
	if err = t.deferImagePositions(ctx); err != nil {
		return err
	}
	result, err := t.tx.ExecContext(ctx, `UPDATE product_color_images i SET position=(ordered.position-1)::smallint
 FROM unnest($2::bigint[]) WITH ORDINALITY ordered(id,position)
 WHERE i.product_color_id=$1 AND i.id=ordered.id`, colorID, pq.Array(ids))
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != int64(len(ids)) {
		return usecase.ErrResourceHidden
	}
	_, err = t.tx.ExecContext(ctx, `SET CONSTRAINTS product_color_images_pkey IMMEDIATE`)
	return err
}
