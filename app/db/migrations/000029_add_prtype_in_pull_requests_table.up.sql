-- Add the pr_type column without a default value
ALTER TABLE pull_requests
ADD COLUMN pr_type VARCHAR(50);

-- Update existing records to set pr_type to 'automated'
UPDATE pull_requests
SET pr_type = 'automated'
WHERE pr_type IS NULL;

-- Alter the column to set NOT NULL constraint
ALTER TABLE pull_requests
MODIFY COLUMN pr_type VARCHAR(50) NOT NULL;
