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

# Test status tracking
TEST_FAILED=false

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

log_failure() {
    echo -e "${RED}[FAILURE]${NC} $1"
    TEST_FAILED=true
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

log_info "Starting UMH Core Protocol Converter lifecycle test..."

# Step 1: Clear the data directory
log_info "Step 1: Clearing umh-core/data directory..."
if [ -d "data" ]; then
    rm -rf data/*
    log_info "Data directory cleared"
else
    log_info "Data directory doesn't exist, will be created by make command"
fi

# Step 2: Start umh-core with empty config
log_info "Step 2: Starting umh-core with empty config..."
make test-empty &
MAKE_PID=$!

# Give the container time to start
log_info "Waiting for container to start..."
# Wait up to 60 seconds for the container to appear in docker ps
CONTAINER_FOUND=false
for i in {1..60}; do
    if docker ps | grep -q umh-core; then
        CONTAINER_FOUND=true
        break
    fi
    sleep 1
done

if [ "$CONTAINER_FOUND" = false ]; then
    log_error "UMH Core container failed to start"
    exit 1
fi

log_info "UMH Core container is running"

# Step 3: Replace config with protocol converter config
log_info "Step 3: Replacing config with protocol converter config..."

# Check if config file exists
if [ ! -f "data/config.yaml" ]; then
    log_error "Config file data/config.yaml not found"
    exit 1
fi

# Create a backup of the current config
cp data/config.yaml data/config.yaml.backup

# Replace config with protocol converter example
cp examples/example-config-pc.yaml data/config.yaml
log_info "Config replaced with protocol converter configuration"

# Step 4: Wait 10 seconds and check for protocol converter logs
log_info "Step 4: Waiting 10 seconds for protocol converter services to create logs..."
sleep 10

# Check if protocol converter logs folder exists
PC_LOGS_FOUND=false
if [ -d "data/logs" ]; then
    log_info "Logs directory exists, checking for protocol converter logs..."
    
    # Look for protocol converter related log folders (could be protocolconverter, protocol-converter, or pc-related)
    PC_LOG_DIRS=$(find data/logs -type d \( -name "*protocolconverter*" -o -name "*protocol-converter*" -o -name "*pc-*" -o -name "*gen*" \) 2>/dev/null || true)
    
    if [ ! -z "$PC_LOG_DIRS" ]; then
        log_info "Found protocol converter log directories:"
        echo "$PC_LOG_DIRS" | while read -r dir; do
            log_info "  - $dir"
        done
        PC_LOGS_FOUND=true
    else
        log_warn "No protocol converter log directories found"
        log_info "Available log directories:"
        find data/logs -type d 2>/dev/null | while read -r dir; do
            log_info "  - $dir"
        done
    fi
else
    log_error "No logs directory found at data/logs"
    exit 1
fi

# Check if protocol converter services exist in /run/service
log_info "Checking for protocol converter services in /run/service..."
PC_SERVICES_FOUND=false
PC_SERVICES=$(docker exec -it umh-core ls /run/service 2>/dev/null | grep -E "(protocolconverter-gen|gen)" || true)

if [ ! -z "$PC_SERVICES" ]; then
    log_info "Found protocol converter services:"
    echo "$PC_SERVICES" | while read -r service; do
        log_info "  - $service"
    done
    PC_SERVICES_FOUND=true
else
    log_warn "No protocol converter services found in /run/service"
    log_info "Available services:"
    docker exec -it umh-core ls /run/service 2>/dev/null | while read -r service; do
        log_info "  - $service"
    done
fi

# Check benthos configuration if services exist
if [ "$PC_SERVICES_FOUND" = true ]; then
    log_info "Checking benthos configuration for protocol converter..."
    BENTHOS_CONFIG_PATH="/run/service/benthos-dataflow-read-protocolconverter-gen/config/benthos.yaml"
    
    if docker exec -it umh-core test -f "$BENTHOS_CONFIG_PATH" 2>/dev/null; then
        log_info "Benthos config file exists, checking for 'generate' string..."
        
        if docker exec -it umh-core grep -q "generate" "$BENTHOS_CONFIG_PATH" 2>/dev/null; then
            log_info "✅ SUCCESS: Found 'generate' in benthos configuration"
        else
            log_failure "⚠️  'generate' string not found in benthos configuration"
            log_info "Benthos config content:"
            docker exec -it umh-core cat "$BENTHOS_CONFIG_PATH" 2>/dev/null || log_error "Failed to read config file"
        fi
    else
        log_warn "Benthos config file not found at $BENTHOS_CONFIG_PATH"
    fi
    
    # Check nmap script configuration
    log_info "Checking nmap script configuration..."
    NMAP_SCRIPT_PATH="/run/service/nmap-connection-protocolconverter-gen/config/run_nmap.sh"
    
    if docker exec -it umh-core test -f "$NMAP_SCRIPT_PATH" 2>/dev/null; then
        log_info "Nmap script file exists, checking for '1.1.1.1' string..."
        
        if docker exec -it umh-core grep -q "1.1.1.1" "$NMAP_SCRIPT_PATH" 2>/dev/null; then
            log_info "✅ SUCCESS: Found '1.1.1.1' in nmap script configuration"
        else
            log_failure "⚠️  '1.1.1.1' string not found in nmap script configuration"
            log_info "Nmap script content:"
            docker exec -it umh-core cat "$NMAP_SCRIPT_PATH" 2>/dev/null || log_error "Failed to read nmap script file"
        fi
    else
        log_warn "Nmap script file not found at $NMAP_SCRIPT_PATH"
    fi
fi

# Step 5: Wait 5 seconds to stabilize
if [ "$PC_LOGS_FOUND" = true ] || [ "$PC_SERVICES_FOUND" = true ]; then
    log_info "Step 5: Protocol converter logs or services found, waiting 5 seconds to stabilize..."
    sleep 5
    
    # Step 6: Replace config back to empty config
    log_info "Step 6: Replacing config back to empty configuration..."
    cp examples/example-config-empty.yaml data/config.yaml
    log_info "Config replaced with empty configuration"
    
    # Step 7: Wait 5 seconds and check that protocol converter logs are cleaned up
    log_info "Step 7: Waiting 15 seconds for protocol converter services to stop and logs to be cleaned up..."
    sleep 15
    
    # Check if protocol converter logs still exist
    PC_LOGS_AFTER=$(find data/logs -type d \( -name "*protocolconverter*" -o -name "*protocol-converter*" -o -name "*pc-*" -o -name "*gen*" \) 2>/dev/null || true)
    
    if [ -z "$PC_LOGS_AFTER" ]; then
        log_info "✅ SUCCESS: Protocol converter log directories have been cleaned up"
    else
        log_failure "⚠️  Protocol converter log directories still exist:"
        echo "$PC_LOGS_AFTER" | while read -r dir; do
            log_warn "  - $dir"
        done
        log_info "This might be expected behavior - some logs may persist until container restart"
    fi
    
    # Check if protocol converter services have been removed from /run/service
    log_info "Checking if protocol converter services have been removed from /run/service..."
    PC_SERVICES_AFTER=$(docker exec -it umh-core ls /run/service 2>/dev/null | grep -E "(protocolconverter-gen|gen)" || true)
    
    if [ -z "$PC_SERVICES_AFTER" ]; then
        log_info "✅ SUCCESS: Protocol converter services have been removed from /run/service"
    else
        log_failure "⚠️  Protocol converter services still exist in /run/service:"
        echo "$PC_SERVICES_AFTER" | while read -r service; do
            log_warn "  - $service"
        done
        log_info "This might indicate a service cleanup issue"
    fi
    
    # Check if benthos processes are still running
    log_info "Checking if benthos processes are still running..."
    BENTHOS_PROCESSES=$(docker exec -it umh-core ps auxsf 2>/dev/null | grep "protocolconverter-gen" | grep -v grep || true)
    
    if [ -z "$BENTHOS_PROCESSES" ]; then
        log_info "✅ SUCCESS: No benthos protocol converter processes found running"
    else
        log_failure "⚠️  Benthos protocol converter processes still running:"
        echo "$BENTHOS_PROCESSES" | while IFS= read -r process; do
            log_warn "  - $process"
        done
        log_failure "This indicates a process cleanup issue - processes should be terminated when services are removed"
    fi
else
    log_warn "No protocol converter logs or services were found initially, skipping config replacement step"
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

# Final test status and log copying
if [ "$TEST_FAILED" = true ]; then
    log_failure "=========================================="
    log_failure "TEST FAILED: One or more checks failed!"
    log_failure "=========================================="
    
    # Copy umh-core logs for investigation
    log_info "Copying umh-core logs to last_logs.txt for investigation..."
    if [ -f "data/logs/umh-core/current" ]; then
        cp "data/logs/umh-core/current" "last_logs.txt"
        log_info "Logs copied to umh-core/last_logs.txt"
    else
        log_warn "umh-core/data/logs/umh-core/current not found, cannot copy logs"
        # Try alternative log locations
        if [ -d "data/logs/umh-core" ]; then
            log_info "Available files in data/logs/umh-core:"
            ls -la data/logs/umh-core/ | while read -r file; do
                log_info "  - $file"
            done
        fi
    fi
else
    log_info "✅ ALL TESTS PASSED!"
fi

log_info "Protocol converter lifecycle test completed!" 