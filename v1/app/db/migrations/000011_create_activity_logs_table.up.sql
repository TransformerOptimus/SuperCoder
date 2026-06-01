-- up.sql
CREATE TABLE activity_logs (
                               id SERIAL PRIMARY KEY,
                               execution_id INTEGER NOT NULL,
                               execution_step_id INTEGER NOT NULL,
                               log_message TEXT NOT NULL,
                               type VARCHAR(50) NOT NULL,
                               created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
                               updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_activity_logs_type ON activity_logs(type);
