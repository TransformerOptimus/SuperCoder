#!/bin/bash

# Wait for PostgreSQL to be ready
while ! pg_isready -h "${AI_DEVELOPER_DB_HOST}" -p 5432 -q; do
    echo "Waiting for PostgreSQL to be ready..."
    sleep 1
done

echo "Running migrations..."
# Check if AI_DEVELOPER_ENV is set to "production"
if [ "${AI_DEVELOPER_ENV}" = "production" ]; then
    # Run migrations in production mode
    migrate -path /go/src/packages/ai-developer/app/db/migrations -database "postgres://${AI_DEVELOPER_DB_USER}:${AI_DEVELOPER_DB_PASSWORD}@${AI_DEVELOPER_DB_HOST}:5432/ai-developer?sslmode=enable" up
else
    # Run migrations in development mode
    migrate -path /go/src/packages/ai-developer/app/db/migrations -database "postgres://${AI_DEVELOPER_DB_USER}:${AI_DEVELOPER_DB_PASSWORD}@${AI_DEVELOPER_DB_HOST}:5432/ai-developer?sslmode=disable" up
fi
