-- Create the organisation_users table
CREATE TABLE organisation_users (
                                    id SERIAL PRIMARY KEY,
                                    user_id INT NOT NULL,
                                    organisation_id INT NOT NULL,
                                    is_active BOOLEAN DEFAULT false,
                                    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
                                    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Create an index on the user_id column
CREATE INDEX idx_organisation_users_user_id ON organisation_users(user_id);

-- Create an index on the organisation_id column
CREATE INDEX idx_organisation_users_organisation_id ON organisation_users(organisation_id);
