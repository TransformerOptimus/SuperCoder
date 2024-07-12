ALTER TABLE pull_requests
ADD COLUMN pr_type VARCHAR(50) NOT NULL DEFAULT 'automated';

-- Update existing rows to have the value 'automated' for prtype
UPDATE pull_requests
SET pr_type = 'automated';

-- Remove the default constraint if it's not needed for future inserts
ALTER TABLE pull_requests
ALTER COLUMN pr_type DROP DEFAULT;
