-- Create "repos" table
CREATE TABLE "public"."repos" (
  "id" bigserial NOT NULL,
  "user_id" character varying(255) NOT NULL,
  "workspace_id" bigint NOT NULL,
  "machine_id" character varying(255) NOT NULL,
  "repo_path" text NOT NULL,
  "repo_url" text NULL,
  "created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "deleted_at" timestamptz NULL,
  PRIMARY KEY ("id")
);
-- Create index "idx_repo_identity" to table: "repos"
CREATE UNIQUE INDEX "idx_repo_identity" ON "public"."repos" ("user_id", "workspace_id", "machine_id", "repo_path");
-- Create index "idx_repos_deleted_at" to table: "repos"
CREATE INDEX "idx_repos_deleted_at" ON "public"."repos" ("deleted_at");
-- Create "shard_assignments" table
CREATE TABLE "public"."shard_assignments" (
  "id" bigserial NOT NULL,
  "repo_id" bigint NOT NULL,
  "collection_name" character varying(255) NOT NULL,
  "created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "deleted_at" timestamptz NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "fk_shard_assignments_repo" FOREIGN KEY ("repo_id") REFERENCES "public"."repos" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "idx_shard_assignments_deleted_at" to table: "shard_assignments"
CREATE INDEX "idx_shard_assignments_deleted_at" ON "public"."shard_assignments" ("deleted_at");
-- Create index "idx_shard_repo_id" to table: "shard_assignments"
CREATE UNIQUE INDEX "idx_shard_repo_id" ON "public"."shard_assignments" ("repo_id");
-- Drop "review_comments" table
DROP TABLE "public"."review_comments";
-- Drop "reviews" table
DROP TABLE "public"."reviews";
-- Drop "repositories" table
DROP TABLE "public"."repositories";
