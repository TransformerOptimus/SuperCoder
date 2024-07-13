-- Add the frontend_framework column without a default value
ALTER TABLE projects
ADD COLUMN frontend_framework VARCHAR(100);

-- Update existing records to set frontend_framework to 'nextjs'
UPDATE projects
SET frontend_framework = 'nextjs'
WHERE frontend_framework IS NULL;

-- Alter the column to set NOT NULL constraint
ALTER TABLE projects
ALTER COLUMN frontend_framework SET NOT NULL;
