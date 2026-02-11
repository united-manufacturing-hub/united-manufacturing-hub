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

: "${AUTH_TOKEN:?Set AUTH_TOKEN to your instance auth token}"
: "${UMH_CORE_IMAGE:?Set UMH_CORE_IMAGE to the umh-core container image}"

echo "=== Scenario 5a: Pull-Only Delays (path-based) ==="
echo "Lognormal delay on /v2/instance/pull: mu=10.0 sigma=0.3 cap=31000ms, 20% kill"
echo ""

docker compose build --quiet chaos-proxy

CHAOS_PROXY_FLAGS="--drop-every=0 --long-poll --long-poll-mu=10.0 --long-poll-sigma=0.3 --long-poll-cap=31000 --long-poll-kill-pct=20 --long-poll-path=/v2/instance/pull" \
  docker compose up
