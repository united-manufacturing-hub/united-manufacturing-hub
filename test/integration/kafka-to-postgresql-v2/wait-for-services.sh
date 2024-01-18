#!/bin/bash

# Wait for TimescaleDB to be ready
echo "Waiting for TimescaleDB to start..."
until docker exec timescaledb pg_isready -U postgres; do
  sleep 1
done
echo "TimescaleDB is ready."

# Check if RedPanda is up
echo "Checking if RedPanda is up..."
until docker exec redpanda bash -c "rpk cluster health | grep -E 'Healthy:.+true' || exit 1"; do
    sleep 1
done
echo "RedPanda is up."
