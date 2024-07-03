CREATE TABLE pull_request_comments (
                                       id SERIAL PRIMARY KEY,
                                       story_id INT NOT NULL,
                                       pull_request_id INT NOT NULL,
                                       comment VARCHAR(255) NOT NULL,
                                       created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
                                       updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_pull_requests_id_comments ON pull_request_comments(pull_request_id);
