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

# Scenario 5b: Push-Only Delays (path-based)
#
# Applies long-poll delays only to requests whose path contains
# /v2/instance/push, leaving pull unaffected. Simulates scenarios
# where the push endpoint is slow (e.g., upstream congestion or
# rate limiting) while pull remains healthy.
#
# Parameters:
#   mu=10.0, sigma=0.3 =>  tighter distribution around ~22s median
#   cap=31000          =>  no delay exceeds 31s
#   kill-pct=20        =>  20% of delayed pushes killed mid-stream
#   path=/v2/instance/push => only push requests are affected
#   drop-every=0       =>  no connection drops
#
# Expected behavior:
#   - Pull works normally with no delays
#   - Push requests experience high latency and occasional kills
#   - Status updates are delayed or lost
#   - Pull worker continues receiving actions normally
#
# Pass criteria:
#   - Pull metrics show normal latency
#   - Push metrics show elevated latency
#   - Instance may appear degraded in Management Console
#   - No goroutine leaks or unbounded queue growth

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR/.."

echo "=== Scenario 5b: Push-Only Delays (path-based) ==="
echo "Lognormal delay on /v2/instance/push: mu=10.0 sigma=0.3 cap=31000ms, 20% kill"
echo ""

docker compose build --quiet chaos-proxy

CHAOS_PROXY_FLAGS="--drop-every=0 --long-poll --long-poll-mu=10.0 --long-poll-sigma=0.3 --long-poll-cap=31000 --long-poll-kill-pct=20 --long-poll-path=/v2/instance/push" \
  docker compose up
