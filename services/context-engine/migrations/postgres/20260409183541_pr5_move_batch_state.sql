-- PR5: move per-batch state from sync_sessions (O(N^2)) to sync_batches (O(1)).
--
-- Per-batch state used to live on sync_sessions in four growing JSONB columns
-- (processed_files, processed_deletes, failed_files, failed_deletes). The
-- MergeBatchResult UPDATE merged each batch's delta with `||` plus
-- jsonb_agg(DISTINCT ...) — O(N) per batch, O(N^2) across a session. A
-- 4576-file sync measured 1227 ms on that single UPDATE.
--
-- This migration moves state to sync_batches as a diff against the existing
-- accepted_files/accepted_deletes manifest, adds a (sync_id, state) index
-- for the finalizer and CheckAllBatchesTerminal read paths, and drops the
-- four obsolete columns from sync_sessions. Storage is minimal on the happy
-- path (~30 bytes per batch) because failed_files/failed_deletes are empty
-- unless the pipeline bisect isolated per-file failures.
--
-- Atlas note: the original diff output also dropped the FKs and partial
-- indexes added by 20260408054600_add_streaming_fks_and_partial_indexes.sql,
-- because GORM tags cannot express WHERE clauses or FK relation fields on
-- these models. Those drops are spurious — the desired state still has those
-- FKs and partial indexes — so they have been removed from this file (same
-- pattern documented in 20260408054600 and huddle's 20260313094200).
--
-- The DO $$ preflight block is hand-merged: Atlas cannot emit "abort if the
-- db isn't in an expected state" guards. WS9 is pre-cutover so no in-flight
-- sessions exist; the guard fails the migration fast if this assumption is
-- violated rather than silently dropping JSONB that might still be needed.

-- Preflight: no non-terminal sync_sessions may exist. Fail-closed —
-- overriding this guard requires editing the migration, not a 2am runbook.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM "public"."sync_sessions"
         WHERE status NOT IN ('done','failed','expired')
    ) THEN
        RAISE EXCEPTION
            'pr5 migration aborted: % non-terminal sync_sessions exist (expected 0, WS9 is pre-cutover)',
            (SELECT count(*) FROM "public"."sync_sessions"
              WHERE status NOT IN ('done','failed','expired'));
    END IF;
END $$;

-- Modify "sync_batches" table: add state enum + diff columns + terminal_at.
ALTER TABLE "public"."sync_batches" ADD CONSTRAINT "chk_sync_batches_state" CHECK ((state)::text = ANY ((ARRAY['pending'::character varying, 'succeeded'::character varying, 'failed'::character varying])::text[])), ADD COLUMN "state" character varying(16) NOT NULL DEFAULT 'pending', ADD COLUMN "failed_files" jsonb NOT NULL DEFAULT '{}', ADD COLUMN "failed_deletes" jsonb NOT NULL DEFAULT '[]', ADD COLUMN "terminal_at" timestamptz NULL;
-- Create index "idx_sync_batches_sync_state" to table: "sync_batches"
CREATE INDEX "idx_sync_batches_sync_state" ON "public"."sync_batches" ("sync_id", "state");
-- Modify "sync_sessions" table: drop obsolete per-batch JSONB columns.
ALTER TABLE "public"."sync_sessions" DROP COLUMN "processed_files", DROP COLUMN "processed_deletes", DROP COLUMN "failed_files", DROP COLUMN "failed_deletes";
