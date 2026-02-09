#!/usr/bin/env bash
# Scenario 1: Connection Drops
#
# Drops every 3rd TCP connection by sending an immediate EOF.
# Tests that the transport workers (push/pull) correctly detect broken
# connections and retry with exponential backoff.
#
# Expected behavior:
#   - Push/pull workers detect EOF and retry
#   - Error counters increment on drops, reset on success
#   - No panics or goroutine leaks
#   - Heartbeat continues despite dropped connections
#
# Pass criteria:
#   - Instance stays "online" in Management Console
#   - Metrics show retry activity (check :2112/metrics)
#   - No unbounded error accumulation

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR/.."

echo "=== Scenario 1: Connection Drops ==="
echo "Dropping every 3rd connection (EOF on TCP)"
echo ""

docker compose build --quiet chaos-proxy

CHAOS_PROXY_FLAGS="--drop-every=3" \
  docker compose up
