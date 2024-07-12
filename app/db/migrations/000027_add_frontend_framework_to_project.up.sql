ALTER TABLE projects
ADD COLUMN frontend_framework VARCHAR(100);

-- Update existing rows with an empty string
UPDATE projects SET frontend_framework = 'nextjs' WHERE frontend_framework IS NULL;

-- Alter the column to NOT NULL
ALTER TABLE projects
ALTER COLUMN frontend_framework SET NOT NULL;
