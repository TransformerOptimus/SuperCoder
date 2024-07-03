ALTER TABLE execution_outputs
DROP COLUMN pull_request_title,
DROP COLUMN pull_request_description,
DROP COLUMN source_sha,
DROP COLUMN merge_target_sha,
DROP COLUMN merge_base_sha,
DROP COLUMN remote_type,
ALTER COLUMN pull_request_id TYPE INT USING pull_request_id::INT;