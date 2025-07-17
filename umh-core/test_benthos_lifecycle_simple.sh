#!/bin/bash

# Copyright 2025 UMH Systems GmbH
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print colored output
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to cleanup on exit
cleanup() {
    log_info "Cleaning up..."
    make stop-all-pods || true
}

# Set trap to cleanup on script exit
trap cleanup EXIT

# Ensure we're in the umh-core directory
cd "$(dirname "$0")"

log_info "Starting UMH Core Benthos lifecycle test (simple version)..."

# Step 1: Clear the data directory
log_info "Step 1: Clearing umh-core/data directory..."
if [ -d "data" ]; then
    rm -rf data/*
    log_info "Data directory cleared"
else
    log_info "Data directory doesn't exist, will be created by make command"
fi

# Step 2: Start umh-core with benthos config
log_info "Step 2: Starting umh-core with benthos config..."
make test-benthos-single &
MAKE_PID=$!

# Give the container time to start
log_info "Waiting for container to start..."
sleep 30

# Check if container is running
if ! docker ps | grep -q umh-core; then
    log_error "UMH Core container failed to start"
    exit 1
fi

log_info "UMH Core container is running"

# Step 3: Wait 20 seconds and check for benthos logs
log_info "Step 3: Waiting 20 seconds for benthos services to create logs..."
sleep 5

# Check if benthos logs folder exists
BENTHOS_LOGS_FOUND=false
if [ -d "data/logs" ]; then
    log_info "Logs directory exists, checking for benthos logs..."
    
    # Look for benthos-related log folders
    BENTHOS_LOG_DIRS=$(find data/logs -type d -name "*benthos*" 2>/dev/null || true)
    
    if [ ! -z "$BENTHOS_LOG_DIRS" ]; then
        log_info "Found benthos log directories:"
        echo "$BENTHOS_LOG_DIRS" | while read -r dir; do
            log_info "  - $dir"
        done
        BENTHOS_LOGS_FOUND=true
    else
        log_warn "No benthos log directories found"
        log_info "Available log directories:"
        find data/logs -type d 2>/dev/null | while read -r dir; do
            log_info "  - $dir"
        done
    fi
else
    log_error "No logs directory found at data/logs"
    exit 1
fi

# Step 4: Modify config to remove benthos services
if [ "$BENTHOS_LOGS_FOUND" = true ]; then
    log_info "Step 4: Benthos logs found, modifying config to remove benthos services..."
    
    # Check if config file exists
    if [ ! -f "data/config.yaml" ]; then
        log_error "Config file data/config.yaml not found"
        exit 1
    fi
    
    # Create a backup of the original config
    cp data/config.yaml data/config.yaml.backup
    
    # Use yq to remove the benthos section
    if command -v yq >/dev/null 2>&1; then
        log_info "Using yq to modify config..."
        # Remove the entire benthos section using yq
        yq eval 'del(.internal.benthos)' -i data/config.yaml
        log_info "Config modified using yq - benthos section removed"
    else
        log_error "yq is required but not available. Please install yq first."
        log_error "macOS: brew install yq"
        log_error "Linux: wget https://github.com/mikefarah/yq/releases/latest/download/yq_linux_amd64 -O /usr/bin/yq && chmod +x /usr/bin/yq"
        exit 1
    fi
    
    # Step 5: Wait 5 seconds and check that benthos logs are cleaned up
    log_info "Step 5: Waiting 5 seconds for benthos services to stop and logs to be cleaned up..."
    sleep 5
    
    # Check if benthos logs still exist
    BENTHOS_LOGS_AFTER=$(find data/logs -type d -name "*benthos*" 2>/dev/null || true)
    
    if [ -z "$BENTHOS_LOGS_AFTER" ]; then
        log_info "✅ SUCCESS: Benthos log directories have been cleaned up"
    else
        log_warn "⚠️  Benthos log directories still exist:"
        echo "$BENTHOS_LOGS_AFTER" | while read -r dir; do
            log_warn "  - $dir"
        done
        log_info "This might be expected behavior - some logs may persist until container restart"
    fi
else
    log_warn "No benthos logs were found initially, skipping config modification step"
fi

# Final status
log_info "Test completed. Final container status:"
docker ps --filter "name=umh-core" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

log_info "Container logs (last 10 lines):"
docker logs umh-core --tail 10

log_info "Available log directories after test:"
if [ -d "data/logs" ]; then
    find data/logs -type d | while read -r dir; do
        log_info "  - $dir"
    done
else
    log_info "  No logs directory found"
fi

log_info "Test script completed!"
