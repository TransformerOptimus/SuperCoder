CREATE TABLE story_test_cases (
                                  id SERIAL PRIMARY KEY,
                                  story_id INT NOT NULL,
                                  test_case TEXT NOT NULL,
                                  created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
                                  updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_test_cases_story_id ON story_test_cases(story_id);
