-- up.sql
CREATE TABLE story_instructions (
                                    id SERIAL PRIMARY KEY,
                                    story_id INTEGER NOT NULL,
                                    instruction TEXT NOT NULL,
                                    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
                                    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_instructions_story_id ON story_instructions(story_id);
