-- up.sql
CREATE TABLE stories (
                         id SERIAL PRIMARY KEY,
                         project_id INTEGER NOT NULL,
                         title VARCHAR(100) NOT NULL,
                         description TEXT,
                         status VARCHAR(50),
                         created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
                         updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_stories_project_id ON stories(project_id);

