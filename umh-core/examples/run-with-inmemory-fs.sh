#!/bin/bash

# UMH Core In-Memory Filesystem Runner
# This script runs UMH Core with optimized tmpfs mounts for S6 services

set -e

# Configuration
IMAGE_NAME="${UMH_CORE_IMAGE:-umh/umh-core:latest}"
CONTAINER_NAME="${UMH_CORE_CONTAINER:-umh-core-inmemory}"
TMPFS_SIZE="${TMPFS_SIZE:-1g}"

# Tmpfs mount configurations
# Format: mount_point:options
TMPFS_MOUNTS=(
    "/run/service:rw,noexec,nosuid,size=256m"
    "/data/services:rw,noexec,nosuid,size=512m" 
    "/data/logs:rw,noexec,nosuid,size=256m"
    "/data/benthos:rw,noexec,nosuid,size=128m"
    "/data/tmp:rw,noexec,nosuid,size=128m"
)

# Build tmpfs arguments for docker run
TMPFS_ARGS=()
for mount in "${TMPFS_MOUNTS[@]}"; do
    TMPFS_ARGS+=(--tmpfs "$mount")
done

echo "Starting UMH Core with in-memory filesystem..."
echo "Image: $IMAGE_NAME"
echo "Container: $CONTAINER_NAME"
echo "Tmpfs mounts:"
for mount in "${TMPFS_MOUNTS[@]}"; do
    echo "  - $mount"
done

# Stop and remove existing container if it exists
if docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
    echo "Stopping and removing existing container..."
    docker stop "$CONTAINER_NAME" >/dev/null 2>&1 || true
    docker rm "$CONTAINER_NAME" >/dev/null 2>&1 || true
fi

# Run the container with tmpfs mounts
docker run -d \
    --name "$CONTAINER_NAME" \
    --restart unless-stopped \
    "${TMPFS_ARGS[@]}" \
    -v umh_persistent_data:/data/persistent \
    -v umh_redpanda_data:/data/redpanda \
    -e TMPFS_SIZE="$TMPFS_SIZE" \
    -p 9644:9644 \
    -p 9092:9092 \
    -p 8080:8080 \
    --memory 2g \
    "$IMAGE_NAME"

echo "Container started successfully!"
echo ""
echo "Useful commands:"
echo "  View logs:        docker logs -f $CONTAINER_NAME"
echo "  Check tmpfs:      docker exec $CONTAINER_NAME df -h"
echo "  Shell access:     docker exec -it $CONTAINER_NAME sh"
echo "  Stop container:   docker stop $CONTAINER_NAME"
echo ""
echo "Memory usage monitoring:"
echo "  Container stats:  docker stats $CONTAINER_NAME"
echo "  Tmpfs usage:      docker exec $CONTAINER_NAME mount | grep tmpfs"

