-- up.sql
CREATE TABLE story_files (
                             id SERIAL PRIMARY KEY,
                             story_id INTEGER NOT NULL,
                             name VARCHAR(100) NOT NULL,
                             file_path TEXT NOT NULL,
                             created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
                             updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);
