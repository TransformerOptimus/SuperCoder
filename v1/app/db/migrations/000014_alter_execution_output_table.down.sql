ALTER TABLE execution_outputs
ADD COLUMN pull_request_title VARCHAR(255) NOT NULL,
ADD COLUMN pull_request_description TEXT NOT NULL,
ADD COLUMN pull_request_id VARCHAR(100) NOT NULL,
ADD COLUMN source_sha VARCHAR(100),
ADD COLUMN merge_target_sha VARCHAR(100),
ADD COLUMN merge_base_sha VARCHAR(100),
ADD COLUMN remote_type VARCHAR(50) NOT NULL;