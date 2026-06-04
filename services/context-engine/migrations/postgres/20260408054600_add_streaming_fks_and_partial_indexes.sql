-- Follow-up to 20260408054535_create_streaming_tables.sql.
--
-- Adds two things that GORM tags cannot express:
--   1. Foreign keys from sync_batches / sync_outbox to sync_sessions. The
--      generator refuses to emit these from an embedded relation field
--      because doing so inverts the direction (it tries to put the FK on
--      sync_sessions pointing at the child tables' bigserial ids).
--   2. Partial indexes for the session lifecycle and outbox dispatcher
--      hot paths. Partial WHERE clauses cannot be expressed in GORM tags;
--      this follow-up migration mirrors the pattern established in
--      services/huddle/migrations/postgres/20260313094200_partial_unique_index_active_huddles.sql.

-- Replace the non-partial expires_at index with a partial one so the TTL GC
-- scan only touches sessions in a non-terminal state.
DROP INDEX IF EXISTS "public"."idx_sync_sessions_expires_at";
CREATE INDEX "idx_sync_sessions_expires_at" ON "public"."sync_sessions" ("expires_at")
  WHERE status IN ('receiving','processing','finalizing');

-- Parent -> child foreign keys with cascade so TTL GC cleanup of a
-- sync_sessions row removes its children in one operation.
ALTER TABLE "public"."sync_batches"
  ADD CONSTRAINT "fk_sync_batches_sync_session"
  FOREIGN KEY ("sync_id") REFERENCES "public"."sync_sessions" ("sync_id")
  ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE "public"."sync_outbox"
  ADD CONSTRAINT "fk_sync_outbox_sync_session"
  FOREIGN KEY ("sync_id") REFERENCES "public"."sync_sessions" ("sync_id")
  ON UPDATE CASCADE ON DELETE CASCADE;

-- Partial indexes that back the dispatcher. idx_sync_outbox_pending is hit
-- by the FOR UPDATE SKIP LOCKED claim query; idx_sync_outbox_enqueuing is
-- hit by the reaper's stuck-row scan.
CREATE INDEX "idx_sync_outbox_pending" ON "public"."sync_outbox" ("id")
  WHERE status = 'pending';

CREATE INDEX "idx_sync_outbox_enqueuing" ON "public"."sync_outbox" ("claimed_at")
  WHERE status = 'enqueuing';
