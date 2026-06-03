-- Create "sync_batches" table
CREATE TABLE "public"."sync_batches" (
  "id" bigserial NOT NULL,
  "sync_id" uuid NOT NULL,
  "batch_id" text NOT NULL,
  "file_count" bigint NOT NULL,
  "byte_count" bigint NOT NULL,
  "accepted_files" jsonb NOT NULL DEFAULT '{}',
  "accepted_deletes" jsonb NOT NULL DEFAULT '[]',
  "created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY ("id")
);
-- Create index "idx_sync_batches_sync_batch" to table: "sync_batches"
CREATE UNIQUE INDEX "idx_sync_batches_sync_batch" ON "public"."sync_batches" ("sync_id", "batch_id");
-- Create index "idx_sync_batches_sync_id" to table: "sync_batches"
CREATE INDEX "idx_sync_batches_sync_id" ON "public"."sync_batches" ("sync_id");
-- Create "sync_outbox" table
CREATE TABLE "public"."sync_outbox" (
  "id" bigserial NOT NULL,
  "sync_id" uuid NOT NULL,
  "batch_id" text NOT NULL,
  "redis_key" text NOT NULL,
  "task_type" character varying(20) NOT NULL,
  "status" character varying(20) NOT NULL,
  "claimed_at" timestamptz NULL,
  "created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "enqueued_at" timestamptz NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "chk_sync_outbox_status" CHECK ((status)::text = ANY ((ARRAY['pending'::character varying, 'enqueuing'::character varying, 'enqueued'::character varying])::text[])),
  CONSTRAINT "chk_sync_outbox_task_type" CHECK ((task_type)::text = ANY ((ARRAY['stream_batch'::character varying, 'finalize'::character varying])::text[]))
);
-- Create index "idx_sync_outbox_sync_batch_task" to table: "sync_outbox"
CREATE UNIQUE INDEX "idx_sync_outbox_sync_batch_task" ON "public"."sync_outbox" ("sync_id", "batch_id", "task_type");
-- Create "sync_sessions" table
CREATE TABLE "public"."sync_sessions" (
  "sync_id" uuid NOT NULL,
  "user_id" character varying(255) NOT NULL,
  "workspace_id" bigint NOT NULL,
  "machine_id" character varying(255) NOT NULL,
  "repo_path" text NOT NULL,
  "repo_id" bigint NOT NULL,
  "collection_name" character varying(255) NOT NULL,
  "github_org_id" character varying(255) NULL,
  "expected_files" jsonb NOT NULL DEFAULT '{}',
  "expected_deletes" jsonb NOT NULL DEFAULT '[]',
  "received_files" jsonb NOT NULL DEFAULT '{}',
  "received_deletes" jsonb NOT NULL DEFAULT '[]',
  "processed_files" jsonb NOT NULL DEFAULT '{}',
  "processed_deletes" jsonb NOT NULL DEFAULT '[]',
  "failed_files" jsonb NOT NULL DEFAULT '{}',
  "failed_deletes" jsonb NOT NULL DEFAULT '[]',
  "batches_seen" jsonb NOT NULL DEFAULT '{}',
  "merkle_version_at_diff" text NULL,
  "failed_reason" text NULL,
  "status" character varying(20) NOT NULL,
  "created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "expires_at" timestamptz NOT NULL,
  "completed_at" timestamptz NULL,
  PRIMARY KEY ("sync_id"),
  CONSTRAINT "fk_sync_sessions_repo" FOREIGN KEY ("repo_id") REFERENCES "public"."repos" ("id") ON UPDATE CASCADE ON DELETE RESTRICT,
  CONSTRAINT "chk_sync_sessions_status" CHECK ((status)::text = ANY ((ARRAY['receiving'::character varying, 'processing'::character varying, 'finalizing'::character varying, 'done'::character varying, 'failed'::character varying, 'expired'::character varying])::text[]))
);
-- Create index "idx_sync_sessions_expires_at" to table: "sync_sessions"
CREATE INDEX "idx_sync_sessions_expires_at" ON "public"."sync_sessions" ("expires_at");
-- Create index "idx_sync_sessions_identity" to table: "sync_sessions"
CREATE INDEX "idx_sync_sessions_identity" ON "public"."sync_sessions" ("user_id", "workspace_id", "machine_id", "repo_path");
-- Create index "idx_sync_sessions_repo_id" to table: "sync_sessions"
CREATE INDEX "idx_sync_sessions_repo_id" ON "public"."sync_sessions" ("repo_id");
