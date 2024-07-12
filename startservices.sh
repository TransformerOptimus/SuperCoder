#!/bin/bash

# Start the services using Docker Compose
docker-compose up -d

# Wait for the PostgreSQL container to be healthy
echo "Waiting for PostgreSQL to be healthy..."
until [ "$(docker inspect -f '{{.State.Health.Status}}' pg)" == "healthy" ]; do
  sleep 1
done

echo "PostgreSQL is healthy and running."
