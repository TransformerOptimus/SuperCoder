ALTER TABLE executions ADD COLUMN re_execution BOOLEAN DEFAULT FALSE NOT NULL;
UPDATE executions SET re_execution = FALSE WHERE re_execution IS NULL;
