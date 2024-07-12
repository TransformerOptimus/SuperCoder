-- Add the pr_type column without a default value
ALTER TABLE pull_requests
ADD COLUMN "type" VARCHAR(50);

-- Update existing records to set pr_type to 'automated'
UPDATE pull_requests
SET "type" = 'automated'
WHERE "type" IS NULL;

-- Alter the column to set NOT NULL constraint
ALTER TABLE pull_requests
ALTER COLUMN "type" SET NOT NULL;
