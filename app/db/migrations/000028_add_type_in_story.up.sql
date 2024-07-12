-- Add the type column without a default value
ALTER TABLE stories
ADD COLUMN type VARCHAR(255);

-- Update existing records to set type to 'backend'
UPDATE stories
SET type = 'backend'
WHERE type IS NULL;

-- Alter the column to set NOT NULL constraint
ALTER TABLE stories
MODIFY COLUMN type VARCHAR(255) NOT NULL;