-- Rename BackendFramework back to Framework
ALTER TABLE projects
RENAME COLUMN backend_framework TO framework;