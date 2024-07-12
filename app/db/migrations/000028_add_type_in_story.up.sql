-- Add the pr_type column without a default value
ALTER TABLE stories
ADD COLUMN "type" VARCHAR(50);

-- Update existing records to set pr_type to 'automated'
UPDATE stories
SET "type" = 'backend'
WHERE "type" IS NULL;

-- Alter the column to set NOT NULL constraint
ALTER TABLE stories
ALTER COLUMN "type" SET NOT NULL;
