-- up.sql
CREATE TABLE execution_steps (
                                 id SERIAL PRIMARY KEY,
                                 execution_id INTEGER NOT NULL,
                                 name VARCHAR(100) NOT NULL,
                                 type VARCHAR(50) NOT NULL,
                                 request JSONB,
                                 response JSONB,
                                 status VARCHAR(50) NOT NULL,
                                 created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
                                 updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);
