CREATE TABLE llm_api_keys (
    id SERIAL PRIMARY KEY,
    organisation_id INT NOT NULL,
    llm_model VARCHAR(100),
    llm_api_key VARCHAR(100) NOT NULL
);

CREATE INDEX idx_llm_model ON llm_api_keys(llm_model);