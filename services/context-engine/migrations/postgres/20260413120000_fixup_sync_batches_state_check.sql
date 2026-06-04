-- Fix-up for 20260409183541: ADD CONSTRAINT appeared before ADD COLUMN
-- in the same ALTER TABLE statement. On PostgreSQL 13/14 the constraint
-- may not have been created. Re-add it idempotently.
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'chk_sync_batches_state'
  ) THEN
    ALTER TABLE "public"."sync_batches"
      ADD CONSTRAINT "chk_sync_batches_state"
      CHECK ((state)::text = ANY ((ARRAY['pending'::character varying, 'succeeded'::character varying, 'failed'::character varying])::text[]));
  END IF;
END $$;
