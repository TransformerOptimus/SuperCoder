CREATE TABLE organisations (
                               id SERIAL PRIMARY KEY,
                               name VARCHAR(100) NOT NULL,
                               description TEXT,
                               created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
                               updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
