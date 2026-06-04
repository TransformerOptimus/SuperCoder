-- Create "organizations" table
CREATE TABLE "organizations" (
  "id" uuid NOT NULL DEFAULT gen_random_uuid(),
  "external_id" character varying(255) NOT NULL,
  "name" character varying(255) NOT NULL,
  "git_provider" character varying(50) NOT NULL,
  "is_paused" boolean NULL DEFAULT false,
  "created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "deleted_at" timestamptz NULL,
  PRIMARY KEY ("id")
);
-- Create index "idx_organizations_deleted_at" to table: "organizations"
CREATE INDEX "idx_organizations_deleted_at" ON "organizations" ("deleted_at");
-- Create index "idx_organizations_external_id" to table: "organizations"
CREATE UNIQUE INDEX "idx_organizations_external_id" ON "organizations" ("external_id");
-- Create "repositories" table
CREATE TABLE "repositories" (
  "id" uuid NOT NULL DEFAULT gen_random_uuid(),
  "org_id" uuid NOT NULL,
  "full_name" character varying(255) NOT NULL,
  "is_active" boolean NULL DEFAULT true,
  "default_branch" character varying(100) NULL DEFAULT 'main',
  "last_indexed_at" timestamptz NULL,
  "created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY ("id"),
  CONSTRAINT "fk_repositories_organization" FOREIGN KEY ("org_id") REFERENCES "organizations" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "idx_repositories_full_name" to table: "repositories"
CREATE UNIQUE INDEX "idx_repositories_full_name" ON "repositories" ("full_name");
-- Create index "idx_repositories_org_id" to table: "repositories"
CREATE INDEX "idx_repositories_org_id" ON "repositories" ("org_id");
-- Create "reviews" table
CREATE TABLE "reviews" (
  "id" uuid NOT NULL DEFAULT gen_random_uuid(),
  "repo_id" uuid NOT NULL,
  "pr_number" bigint NOT NULL,
  "pr_url" text NOT NULL,
  "author_handle" character varying(100) NULL,
  "verdict" character varying(20) NULL,
  "comment_count" bigint NULL DEFAULT 0,
  "base_sha" character varying(40) NULL,
  "head_sha" character varying(40) NULL,
  "created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY ("id"),
  CONSTRAINT "fk_reviews_repository" FOREIGN KEY ("repo_id") REFERENCES "repositories" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "idx_reviews_repo_id" to table: "reviews"
CREATE INDEX "idx_reviews_repo_id" ON "reviews" ("repo_id");
-- Create "review_comments" table
CREATE TABLE "review_comments" (
  "id" uuid NOT NULL DEFAULT gen_random_uuid(),
  "review_id" uuid NOT NULL,
  "file_path" text NOT NULL,
  "line_number" bigint NULL,
  "severity" character varying(10) NULL,
  "category" character varying(50) NULL,
  "issue_type_id" character varying(100) NULL,
  "created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY ("id"),
  CONSTRAINT "fk_reviews_comments" FOREIGN KEY ("review_id") REFERENCES "reviews" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "idx_review_comments_review_id" to table: "review_comments"
CREATE INDEX "idx_review_comments_review_id" ON "review_comments" ("review_id");
