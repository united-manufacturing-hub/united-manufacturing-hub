#!/usr/bin/env bash

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

# Scenario 2: Long-Poll Delays
#
# Holds connections for a lognormal-distributed delay before proxying.
# Simulates high-latency networks, packet inspection appliances, and
# overloaded upstream proxies.
#
# Parameters:
#   mu=8.5, sigma=1.2  =>  median ~4.9s (e^8.5), with occasional 20-30s outliers
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

: "${AUTH_TOKEN:?Set AUTH_TOKEN to your instance auth token}"
: "${UMH_CORE_IMAGE:?Set UMH_CORE_IMAGE to the umh-core container image}"

trap 'docker compose down 2>/dev/null' EXIT

echo "=== Scenario 2: Long-Poll Delays ==="
echo "Lognormal delay: mu=8.5 sigma=1.2 cap=31000ms, 20% mid-stream kill"
echo ""

docker compose build --quiet chaos-proxy

CHAOS_PROXY_FLAGS="--drop-every=0 --long-poll --long-poll-mu=8.5 --long-poll-sigma=1.2 --long-poll-cap=31000 --long-poll-kill-pct=20" \
  docker compose up
