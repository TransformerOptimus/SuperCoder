-- up.sql
CREATE TABLE executions (
                            id SERIAL PRIMARY KEY,
                            story_id INTEGER NOT NULL,
                            plan TEXT,
                            status VARCHAR(100) NOT NULL,
                            branch_name VARCHAR(100) NOT NULL,
                            git_commit_id VARCHAR(100),
                            instruction TEXT NOT NULL,
                            created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
                            updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);
