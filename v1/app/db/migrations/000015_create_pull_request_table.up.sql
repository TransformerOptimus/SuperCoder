CREATE TABLE pull_requests (
                                                id SERIAL PRIMARY KEY,
                                                story_id INT NOT NULL,
                                                execution_output_id INT NOT NULL,
                                                pull_request_title VARCHAR(255) NOT NULL,
                                                pull_request_number INT NOT NULL,
                                                status VARCHAR(255) NOT NULL,
                                                pull_request_description TEXT NOT NULL,
                                                pull_request_id VARCHAR(100) NOT NULL,
                                                source_sha VARCHAR(100),
                                                merge_target_sha VARCHAR(100),
                                                merge_base_sha VARCHAR(100),
                                                remote_type VARCHAR(50) NOT NULL,
                                                created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
                                                updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
                                                merged_at TIMESTAMP WITH TIME ZONE,
                                                closed_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_story_pull_requests ON pull_requests(story_id);
