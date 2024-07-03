CREATE TABLE execution_files (
                                 id SERIAL PRIMARY KEY,
                                 execution_id INT NOT NULL,
                                 execution_step_id INT NOT NULL,
                                 file_path TEXT NOT NULL,
                                 created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
                                 updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
