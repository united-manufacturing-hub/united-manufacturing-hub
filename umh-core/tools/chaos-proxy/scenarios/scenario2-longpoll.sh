#!/usr/bin/env bash
# Scenario 2: Long-Poll Delays
#
# Holds connections for a lognormal-distributed delay before proxying.
# Simulates high-latency networks, packet inspection appliances, and
# overloaded upstream proxies.
#
# Parameters:
#   mu=8.5, sigma=1.2  =>  median ~5s, with occasional 20-30s outliers
#   cap=31000          =>  no delay exceeds 31s (just over HTTP timeout)
#   kill-pct=20        =>  20% of delayed connections are killed mid-wait
#
# Expected behavior:
#   - Transport workers handle slow responses gracefully
#   - Context deadlines fire for requests exceeding timeout
#   - Mid-stream kills are detected and retried
#   - System remains stable under sustained slow responses
#
# Pass criteria:
#   - Instance stays "online" (may show degraded heartbeat)
#   - No goroutine leaks from stuck requests
#   - Metrics show elevated latency, not errors

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR/.."

echo "=== Scenario 2: Long-Poll Delays ==="
echo "Lognormal delay: mu=8.5 sigma=1.2 cap=31000ms, 20% mid-stream kill"
echo ""

docker compose build --quiet chaos-proxy

CHAOS_PROXY_FLAGS="--drop-every=0 --long-poll --long-poll-mu=8.5 --long-poll-sigma=1.2 --long-poll-cap=31000 --long-poll-kill-pct=20" \
  docker compose up
