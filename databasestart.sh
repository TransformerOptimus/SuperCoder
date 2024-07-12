#!/bin/bash

# Check if Docker is installed
if ! [ -x "$(command -v docker)" ]; then
  echo 'Error: docker is not installed.' >&2
  exit 1
fi

# Check if Docker Compose is installed
if ! [ -x "$(command -v docker-compose)" ]; then
  echo 'Error: docker-compose is not installed.' >&2
  exit 1
fi

# Pull the latest postgres image
docker pull postgres:16.3-alpine3.20

# Start the PostgreSQL container using Docker Compose
docker-compose up -d

# Wait for the PostgreSQL container to be healthy
echo "Waiting for PostgreSQL to be healthy..."
until [ "$(docker inspect -f '{{.State.Health.Status}}' pg)" == "healthy" ]; do
  sleep 1
done

echo "PostgreSQL is healthy and running."

# Optional: Display logs
docker-compose logs -f pg
