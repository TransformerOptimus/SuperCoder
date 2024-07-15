CREATE TABLE design_story_reviews (
                                       id SERIAL PRIMARY KEY,
                                       story_id INT NOT NULL,
                                       comment TEXT NOT NULL,
                                       created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
                                       updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_design_story_review_comments ON design_story_reviews(story_id);