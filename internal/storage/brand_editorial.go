package storage

import (
	"context"
	"slices"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"srez/internal/entities"
)

func (m *managementTx) BrandEditorialMedia(ctx context.Context) ([]int64, error) {
	ids := []int64{}
	err := m.tx.SelectContext(ctx, &ids, `SELECT asset_id FROM brand_editorial_media WHERE brand_id=$1 ORDER BY asset_id DESC`, m.brandID)
	return ids, err
}

func (m *managementTx) BindBrandEditorialMedia(ctx context.Context, id int64) error {
	_, err := m.tx.ExecContext(ctx, `INSERT INTO brand_editorial_media (asset_id, brand_id) VALUES ($1,$2)`, id, m.brandID)
	return err
}

func (m *managementTx) replaceBrandPageMedia(ctx context.Context, state entities.BrandPageEditorState) error {
	draft := state.Draft.MediaIDs()
	var published []int64
	if state.Published != nil {
		published = state.Published.MediaIDs()
	}
	requested := append(append([]int64{}, draft...), published...)
	slices.Sort(requested)
	requested = slices.Compact(requested)
	// Asset locks serialize retention with save/publish. The existing brand lock
	// serializes these mutations with authorization and finalize. GC never locks a brand.
	oldAndNew := []int64{}
	err := m.tx.SelectContext(ctx, &oldAndNew, `
  SELECT a.id FROM media_assets a
  WHERE a.id IN (SELECT asset_id FROM brand_page_media WHERE brand_id=$1)
     OR (a.id=ANY($2) AND a.brand_id=$1 AND a.purpose='brand_editorial_image' AND a.state='ready'
      AND EXISTS(SELECT 1 FROM brand_editorial_media l WHERE l.asset_id=a.id AND l.brand_id=$1))
  ORDER BY a.id FOR UPDATE`, m.brandID, pq.Array(requested))
	if err != nil {
		return err
	}
	// A renewed SQL snapshot checks the binding after any wait on GC.
	var count int
	if err = m.tx.GetContext(ctx, &count, `SELECT count(*) FROM brand_editorial_media l JOIN media_assets a ON a.id=l.asset_id
 WHERE l.brand_id=$1 AND a.brand_id=$1 AND a.state='ready' AND a.purpose='brand_editorial_image'
 AND l.asset_id=ANY($2) AND l.asset_id=ANY($3)`, m.brandID, pq.Array(requested), pq.Array(oldAndNew)); err != nil {
		return err
	}
	if count != len(requested) {
		return entities.ErrInvalidValue
	}
	if _, err = m.tx.ExecContext(ctx, `UPDATE brand_editorial_media SET retain_until=now()+interval '7 days'
 WHERE brand_id=$1 AND asset_id IN (SELECT asset_id FROM brand_page_media WHERE brand_id=$1)
 AND NOT (asset_id=ANY($2))`, m.brandID, pq.Array(requested)); err != nil {
		return err
	}
	if _, err = m.tx.ExecContext(ctx, `DELETE FROM brand_page_media WHERE brand_id=$1`, m.brandID); err != nil {
		return err
	}
	_, err = m.tx.ExecContext(ctx, `INSERT INTO brand_page_media (brand_id, published, asset_id)
 SELECT $1::bigint, false, unnest(COALESCE($2::bigint[], '{}'))
 UNION ALL SELECT $1::bigint, true, unnest(COALESCE($3::bigint[], '{}'))`, m.brandID, pq.Array(draft), pq.Array(published))
	return err
}

func expireEditorialMedia(ctx context.Context, tx *sqlx.Tx) error {
	var ids []int64
	err := tx.SelectContext(ctx, &ids, `SELECT l.asset_id FROM brand_editorial_media l JOIN media_assets a ON a.id=l.asset_id
 WHERE l.retain_until<now() AND NOT EXISTS(SELECT 1 FROM brand_page_media s WHERE s.asset_id=l.asset_id)
 ORDER BY a.id LIMIT 100 FOR UPDATE OF a SKIP LOCKED`)
	if err != nil || len(ids) == 0 {
		return err
	}
	// Recheck with a fresh snapshot after acquiring asset locks. The library
	// detach trigger moves unreferenced assets into the existing five-minute GC queue.
	_, err = tx.ExecContext(ctx, `DELETE FROM brand_editorial_media l WHERE l.asset_id=ANY($1)
 AND l.retain_until<now() AND NOT EXISTS(SELECT 1 FROM brand_page_media s WHERE s.asset_id=l.asset_id)`, pq.Array(ids))
	return err
}
