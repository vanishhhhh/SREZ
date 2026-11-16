package storage

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"srez/internal/entities"
	"srez/internal/usecase"
)

type Media struct{ db *sqlx.DB }

func NewMedia(db *sqlx.DB) *Media { return &Media{db} }

const assetColumns = `id,initiated_by,purpose,COALESCE(target_user_id,0) AS target_user_id,COALESCE(brand_id,0) AS brand_id,COALESCE(product_id,0) AS product_id,COALESCE(color_id,0) AS color_id,bucket,object_key,state,COALESCE(mime,'') AS mime,COALESCE(size_bytes,0) AS size_bytes,created_at,confirmed_at,expires_at,delete_after`
const uploadColumns = `id,asset_id,initiated_by,expected_size,expected_mime,source_size,image_id,position,state,created_at,completed_at`

func mediaMissing(e error) error {
	if errors.Is(e, sql.ErrNoRows) {
		return usecase.ErrResourceHidden
	}
	return e
}
func loadMediaUpload(ctx context.Context, q sqlx.ExtContext, uid, id int64, lock bool) (*usecase.MediaUpload, error) {
	var u usecase.MediaUpload
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	e := sqlx.GetContext(ctx, q, &u, `SELECT `+uploadColumns+` FROM media_upload_requests WHERE id=$1 AND initiated_by=$2`+suffix, id, uid)
	if e != nil {
		return nil, mediaMissing(e)
	}
	e = sqlx.GetContext(ctx, q, &u.Asset, `SELECT `+assetColumns+` FROM media_assets WHERE id=$1`+suffix, u.AssetID)
	if e != nil {
		return nil, mediaMissing(e)
	}
	return &u, nil
}
func (s *Media) Upload(ctx context.Context, uid, id int64) (*usecase.MediaUpload, error) {
	return loadMediaUpload(ctx, s.db, uid, id, false)
}
func (s *Media) WithinMediaUser(ctx context.Context, uid int64, fn func(usecase.MediaTx, *entities.User) error) error {
	tx, e := s.db.BeginTxx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var r userRow
	if e = tx.GetContext(ctx, &r, `SELECT `+userColumns+` FROM users WHERE id=$1 FOR NO KEY UPDATE`, uid); e != nil {
		return mediaMissing(e)
	}
	u, e := r.user()
	if e != nil {
		return e
	}
	if e = fn(&managementTx{tx: tx}, u); e != nil {
		return e
	}
	return tx.Commit()
}
func (t *managementTx) CreateMediaUpload(ctx context.Context, u *usecase.MediaUpload) error {
	var count int
	// Parent locks serialize claims within an entity. Also serialize per initiator
	// without taking another user's row lock (brand -> membership -> product order).
	if _, e := t.tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(90410,($1 % 2147483647)::int)`, u.InitiatedBy); e != nil {
		return e
	}
	if e := t.tx.GetContext(ctx, &count, `SELECT count(*) FROM media_upload_requests r JOIN media_assets a ON a.id=r.asset_id WHERE r.initiated_by=$1 AND r.state IN ('pending','processing') AND a.expires_at>now()`, u.InitiatedBy); e != nil {
		return e
	}
	if count >= 20 {
		return usecase.ErrMediaBusy
	}
	a := &u.Asset
	e := t.tx.QueryRowxContext(ctx, `INSERT INTO media_assets(initiated_by,purpose,target_user_id,brand_id,product_id,color_id,bucket,object_key) VALUES($1,$2,NULLIF($3,0),NULLIF($4,0),NULLIF($5,0),NULLIF($6,0),$7,$8) RETURNING id,state,created_at,expires_at`, u.InitiatedBy, a.Purpose, a.TargetUserID, a.BrandID, a.ProductID, a.ColorID, a.Bucket, a.ObjectKey).Scan(&a.ID, &a.State, &a.CreatedAt, &a.ExpiresAt)
	if e != nil {
		return e
	}
	u.AssetID = a.ID
	return t.tx.QueryRowxContext(ctx, `INSERT INTO media_upload_requests(asset_id,initiated_by,expected_size,expected_mime,image_id,position) VALUES($1,$2,$3,$4,$5,$6) RETURNING id,state,created_at`, u.AssetID, u.InitiatedBy, u.ExpectedSize, u.ExpectedMIME, u.ImageID, u.Position).Scan(&u.ID, &u.State, &u.CreatedAt)
}
func (t *managementTx) LockMediaUpload(ctx context.Context, uid, id int64) (*usecase.MediaUpload, error) {
	return loadMediaUpload(ctx, t.tx, uid, id, true)
}
func (t *managementTx) ClaimMediaUpload(ctx context.Context, id int64) error {
	return mediaChanged(t.tx.ExecContext(ctx, `UPDATE media_upload_requests SET state='processing' WHERE id=$1 AND state='pending'`, id))
}
func mediaChanged(r sql.Result, e error) error {
	if e != nil {
		return e
	}
	n, e := r.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return usecase.ErrConflict
	}
	return nil
}
func (t *managementTx) ReadyMediaUpload(ctx context.Context, u *usecase.MediaUpload) error {
	e := mediaChanged(t.tx.ExecContext(ctx, `UPDATE media_assets SET state='ready',mime=$2,size_bytes=$3,confirmed_at=now() WHERE id=$1 AND state='processing' AND expires_at>now()`, u.AssetID, u.Asset.MIME, u.Asset.Size))
	if e != nil {
		return e
	}
	return mediaChanged(t.tx.ExecContext(ctx, `UPDATE media_upload_requests SET state='complete',source_size=$2,image_id=$3,completed_at=now() WHERE id=$1 AND state='processing'`, u.ID, u.SourceSize, u.ImageID))
}
func (t *managementTx) SaveMediaUser(ctx context.Context, u entities.User) error {
	return changedRow(t.tx.ExecContext(ctx, `UPDATE users SET avatar_url=$2,avatar_media_id=NULLIF($3,0),updated_at=$4 WHERE id=$1`, u.ID(), u.AvatarURL(), u.AvatarMediaID(), u.UpdatedAt()))
}
func (t *managementTx) SaveMediaBrand(ctx context.Context, b entities.Brand) error {
	if b.ID() != t.brandID {
		return usecase.ErrResourceHidden
	}
	return changedRow(t.tx.ExecContext(ctx, `UPDATE brands SET logo_url=$2,banner_url=$3,logo_media_id=NULLIF($4,0),banner_media_id=NULLIF($5,0),updated_at=$6 WHERE id=$1`, b.ID(), b.LogoURL(), b.BannerURL(), b.LogoMediaID(), b.BannerMediaID(), b.UpdatedAt()))
}
func (s *Media) RejectUpload(ctx context.Context, id int64) error {
	// Rejected upload keys have never been written by validation failures. Keep a
	// tombstone anyway: failure/timeout reconciliation is shared and idempotent.
	_, e := s.db.ExecContext(ctx, `UPDATE media_upload_requests SET state='rejected' WHERE id=$1 AND state='processing'`, id)
	return e
}
func (s *Media) ReadableAsset(ctx context.Context, uid, id int64) (*usecase.MediaAsset, error) {
	// One SQL snapshot authorizes by the CURRENT live reference and entity state.
	var a usecase.MediaAsset
	e := s.db.GetContext(ctx, &a, `SELECT `+assetColumns+` FROM media_assets a WHERE a.id=$1 AND a.state='ready' AND EXISTS(
 SELECT 1 FROM media_references r LEFT JOIN brands b ON b.id=r.brand_id LEFT JOIN products p ON p.id=r.product_id
 WHERE r.asset_id=a.id AND r.purpose=a.purpose AND (
 (r.purpose='user_avatar' AND r.user_id=$2 AND $2>0) OR
 (r.purpose IN ('brand_logo','brand_banner') AND b.status='active') OR
 (r.purpose='product_image' AND b.status='active' AND p.status='published') OR
 (r.purpose<>'user_avatar' AND b.status<>'suspended' AND EXISTS(SELECT 1 FROM brand_memberships m WHERE m.brand_id=r.brand_id AND m.user_id=$2 AND m.status='active' AND m.role IN ('owner','editor')))))`, id, uid)
	return &a, mediaMissing(e)
}
func (s *Media) Garbage(ctx context.Context) ([]usecase.MediaAsset, error) {
	tx, e := s.db.BeginTxx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	// Do not lock parents. GC only terminalizes expired UNREFERENCED assets.
	// Finishing writers lock their parent, request then asset and recheck expiry.
	var ids []int64
	e = tx.SelectContext(ctx, &ids, `SELECT id FROM media_assets a WHERE ((state='processing' AND expires_at<now()) OR (state='deleting' AND delete_after<now()) ) AND NOT EXISTS(SELECT 1 FROM media_references r WHERE r.asset_id=a.id) ORDER BY COALESCE(delete_after,expires_at),id LIMIT 100 FOR UPDATE SKIP LOCKED`)
	if e != nil {
		return nil, e
	}
	// Reconciliation has its own small budget and finite 24h window. Historical
	// rows never displace the mandatory 100-slot queue or extend their window.
	var resweeps []int64
	if e = tx.SelectContext(ctx, &resweeps, `SELECT id FROM media_assets a WHERE state='deleted' AND reconcile_until>now() AND resweep_after<now() AND NOT EXISTS(SELECT 1 FROM media_references r WHERE r.asset_id=a.id) ORDER BY reconcile_until,resweep_after,id LIMIT 10 FOR UPDATE SKIP LOCKED`); e != nil {
		return nil, e
	}
	ids = append(ids, resweeps...)
	rows := make([]usecase.MediaAsset, 0, len(ids))
	for _, id := range ids {
		if _, e = tx.ExecContext(ctx, `UPDATE media_assets SET state='deleting',delete_after=CASE WHEN state='processing' THEN expires_at WHEN state='deleted' THEN now() ELSE delete_after END WHERE id=$1`, id); e != nil {
			return nil, e
		}
		var a usecase.MediaAsset
		if e = tx.GetContext(ctx, &a, `SELECT `+assetColumns+` FROM media_assets WHERE id=$1`, id); e != nil {
			return nil, e
		}
		rows = append(rows, a)
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	// Update request state only AFTER releasing asset locks. Writers lock request
	// before asset; doing this inside GC's asset transaction would invert order.
	if len(ids) > 0 {
		if _, e = s.db.ExecContext(ctx, `UPDATE media_upload_requests SET state='rejected' WHERE asset_id=ANY($1) AND state IN ('pending','processing')`, pq.Array(ids)); e != nil {
			return nil, e
		}
	}
	return rows, nil
}
func (s *Media) Deleted(ctx context.Context, id int64) error {
	// Recheck hourly for 24h after the first successful deletion, never extend
	// that window on resweep. Terminal assets cannot be attached again.
	_, e := s.db.ExecContext(ctx, `UPDATE media_assets SET state='deleted',delete_after=NULL,deleted_at=COALESCE(deleted_at,now()),reconcile_until=COALESCE(reconcile_until,now()+interval '24 hours'),resweep_after=now()+interval '1 hour' WHERE id=$1 AND state IN ('deleting','deleted')`, id)
	return e
}
func (s *Media) CleanupLegacy(ctx context.Context, objects usecase.MediaObjects, bucket string) error {
	_, e := s.db.ExecContext(ctx, `UPDATE media_uploads SET status='rejected',confirmed_at=NULL,cleaned=false WHERE (status='ready' AND retired_at<now()-interval '7 days') OR (status='pending' AND expires_at<now())`)
	if e != nil {
		return e
	}
	var rows []struct {
		ID      int64  `db:"id"`
		Bucket  string `db:"bucket"`
		Staging string `db:"staging_key"`
		Key     string `db:"object_key"`
		Status  string `db:"status"`
	}
	e = s.db.SelectContext(ctx, &rows, `SELECT id,bucket,staging_key,object_key,status FROM media_uploads WHERE next_cleanup_at<now() AND (NOT cleaned OR expires_at>now()-interval '24 hours') ORDER BY next_cleanup_at,id LIMIT 100`)
	if e != nil {
		return e
	}
	var failures []error
	for _, r := range rows {
		if r.Bucket != bucket {
			failures = append(failures, usecase.ErrMediaBucketMismatch)
			continue
		}
		if e = objects.Delete(ctx, r.Staging); e != nil {
			failures = append(failures, e)
			continue
		}
		if r.Status == "rejected" {
			if e = objects.Delete(ctx, r.Key); e != nil {
				failures = append(failures, e)
				continue
			}
		}
		if _, e = s.db.ExecContext(ctx, `UPDATE media_uploads SET next_cleanup_at=now()+interval '1 hour',cleaned=true WHERE id=$1`, r.ID); e != nil {
			failures = append(failures, e)
		}
	}
	return errors.Join(failures...)
}

// Keep SQL deadlines and data error handling at the existing usecase boundaries.
var _ usecase.MediaRepository = (*Media)(nil)
var _ usecase.MediaTx = (*managementTx)(nil)

// Backlog measures only actionable work, not retained audit history.
func (s *Media) GCBacklog(ctx context.Context) (usecase.MediaGCBacklog, error) {
	var b usecase.MediaGCBacklog
	e := s.db.GetContext(ctx, &b, `SELECT count(*) FILTER(WHERE (state='processing' AND expires_at<now()) OR (state='deleting' AND delete_after<now())) AS pending,COALESCE(max(EXTRACT(EPOCH FROM now()-CASE WHEN state='processing' THEN expires_at ELSE delete_after END)) FILTER(WHERE (state='processing' AND expires_at<now()) OR (state='deleting' AND delete_after<now())),0) AS oldest_seconds,count(*) FILTER(WHERE state='deleted' AND reconcile_until>now() AND resweep_after<now()) AS reconciliation FROM media_assets a WHERE ((state='processing' AND expires_at<now()) OR (state='deleting' AND delete_after<now()) OR (state='deleted' AND reconcile_until>now() AND resweep_after<now())) AND NOT EXISTS(SELECT 1 FROM media_references r WHERE r.asset_id=a.id)`)
	return b, e
}
