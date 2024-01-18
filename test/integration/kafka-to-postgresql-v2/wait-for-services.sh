#!/bin/bash

# Wait for TimescaleDB to be ready
echo "Waiting for TimescaleDB to start..."
timeout=0
until ((timeout == 10)) || docker exec timescaledb pg_isready -U postgres; do
    sleep 1
    ((timeout++))
done
if ((timeout == 10)); then
    echo "Timed out waiting for TimescaleDB to start."
    exit 1
fi
echo "TimescaleDB is ready."

# Check if RedPanda is up
echo "Checking if RedPanda is up..."
timeout=0
until ((timeout == 10)) || docker exec redpanda bash -c "rpk cluster health | grep -E 'Healthy:.+true'"; do
    sleep 1
    ((timeout++))
done
if ((timeout == 10)); then
    echo "Timed out waiting for RedPanda to start."
    exit 1
fi
echo "RedPanda is up."
