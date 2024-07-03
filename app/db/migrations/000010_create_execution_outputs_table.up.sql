CREATE TABLE execution_outputs (
                                   id SERIAL PRIMARY KEY,
                                   execution_id INT NOT NULL,
                                   pull_request_title VARCHAR(255) NOT NULL,
                                   pull_request_description TEXT NOT NULL,
                                   pull_request_id VARCHAR(100) NOT NULL,
                                   source_sha VARCHAR(100),
                                   merge_target_sha VARCHAR(100),
                                   merge_base_sha VARCHAR(100),
                                   remote_type VARCHAR(50) NOT NULL,
                                   created_at TIMESTAMP WITH TIME ZONE DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'),
                                   updated_at TIMESTAMP WITH TIME ZONE DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
);
