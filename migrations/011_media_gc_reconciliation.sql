-- Append-only: keep audit history, but stop perpetual deleted-object DELETEs.
ALTER TABLE media_assets ADD COLUMN deleted_at TIMESTAMPTZ;
ALTER TABLE media_assets ADD COLUMN reconcile_until TIMESTAMPTZ;
ALTER TABLE media_assets ADD COLUMN resweep_after TIMESTAMPTZ;
-- Existing terminal rows receive one finite compatibility window at upgrade.
UPDATE media_assets SET deleted_at=now(),reconcile_until=now()+interval '24 hours',resweep_after=COALESCE(delete_after,now()),delete_after=NULL WHERE state='deleted';
CREATE INDEX media_gc_processing ON media_assets(expires_at,id) WHERE state='processing';
CREATE INDEX media_gc_deleting ON media_assets(delete_after,id) WHERE state='deleting';
CREATE INDEX media_gc_resweep ON media_assets(reconcile_until,resweep_after,id) WHERE state='deleted';
