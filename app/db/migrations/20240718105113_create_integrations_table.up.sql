CREATE TABLE integrations
(
    id               SERIAL PRIMARY KEY,
    created_at       TIMESTAMP    NOT NULL,
    updated_at       TIMESTAMP    NOT NULL,
    deleted_at       TIMESTAMP,
    user_id          BIGINT       NOT NULL,
    integration_type VARCHAR(255) NOT NULL,
    access_token     VARCHAR(255) NOT NULL,
    refresh_token    VARCHAR(255),
    metadata         JSONB
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_integration ON integrations (user_id, integration_type);
CREATE INDEX IF NOT EXISTS idx_user ON integrations (user_id);