#!/usr/bin/env bash
# Scenario 3: Mid-Stream Kills
#
# Kills 30% of connections mid-stream by closing the TCP socket while
# the request is in flight. Simulates NAT table resets, firewall state
# expiration, and Cloudflare edge disconnects.
#
# Parameters:
#   mu=8.5, sigma=1.2  =>  delays before kill follow lognormal distribution
#   kill-pct=30        =>  30% of connections killed (higher than scenario 2)
#   drop-every=0       =>  no additional connection drops
#
# Expected behavior:
#   - Transport workers detect broken pipe / connection reset
#   - Partial responses are discarded, not processed
#   - Retry logic kicks in with backoff
#   - No data corruption from half-received responses
#
# Pass criteria:
#   - Instance recovers after each kill
#   - No panic from reading closed connections
#   - Metrics show kill-related errors that recover

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR/.."

echo "=== Scenario 3: Mid-Stream Kills ==="
echo "Lognormal delay: mu=8.5 sigma=1.2, 30% mid-stream kill"
echo ""

docker compose build --quiet chaos-proxy

CHAOS_PROXY_FLAGS="--drop-every=0 --long-poll --long-poll-mu=8.5 --long-poll-sigma=1.2 --long-poll-kill-pct=30" \
  docker compose up
