#!/usr/bin/env bash
# Scenario 5a: Pull-Only Delays (path-based)
#
# Applies long-poll delays only to requests whose path contains
# /v2/instance/pull, leaving push unaffected. Simulates scenarios
# where the pull endpoint is slow (e.g., backend under load) while
# push remains healthy.
#
# Parameters:
#   mu=10.0, sigma=0.3 =>  tighter distribution around ~22s median
#   cap=31000          =>  no delay exceeds 31s
#   kill-pct=20        =>  20% of delayed pulls killed mid-stream
#   path=/v2/instance/pull => only pull requests are affected
#   drop-every=0       =>  no connection drops
#
# Expected behavior:
#   - Push (POST) works normally with no delays
#   - Pull requests experience high latency and occasional kills
#   - Heartbeat push remains healthy
#   - Pull worker backs off and retries
#
# Pass criteria:
#   - Push metrics show normal latency
#   - Pull metrics show elevated latency
#   - Instance stays "online" via healthy push path
#   - No starvation of pull channel

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR/.."

echo "=== Scenario 5a: Pull-Only Delays (path-based) ==="
echo "Lognormal delay on /v2/instance/pull: mu=10.0 sigma=0.3 cap=31000ms, 20% kill"
echo ""

docker compose build --quiet chaos-proxy

CHAOS_PROXY_FLAGS="--drop-every=0 --long-poll --long-poll-mu=10.0 --long-poll-sigma=0.3 --long-poll-cap=31000 --long-poll-kill-pct=20 --long-poll-path=/v2/instance/pull" \
  docker compose up
