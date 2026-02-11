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

# Scenario 4: Combined Chaos
#
# Combines connection drops, long-poll delays, and mid-stream kills.
# This is the most aggressive scenario and represents a worst-case
# network environment.
#
# Parameters:
#   drop-every=3       =>  every 3rd connection gets immediate EOF
#   long-poll enabled  =>  lognormal delays (mu=8.5, sigma=1.2, cap=31000)
#   kill-pct=20        =>  20% of delayed connections killed mid-stream
#
# Expected behavior:
#   - All chaos types occur simultaneously
#   - Transport workers handle mixed failure modes
#   - System does not enter unrecoverable state
#   - Recovery happens within bounded time after chaos stops
#
# Pass criteria:
#   - Instance eventually reconnects after extended chaos
#   - No deadlocks or goroutine leaks
#   - Error counters are bounded (not growing unbounded)
#   - Metrics endpoint (:2112) remains responsive

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR/.."

: "${AUTH_TOKEN:?Set AUTH_TOKEN to your instance auth token}"
: "${UMH_CORE_IMAGE:?Set UMH_CORE_IMAGE to the umh-core container image}"

echo "=== Scenario 4: Combined Chaos ==="
echo "Drops every 3rd + lognormal delay (mu=8.5 sigma=1.2 cap=31000ms) + 20% kill"
echo ""

docker compose build --quiet chaos-proxy

CHAOS_PROXY_FLAGS="--drop-every=3 --long-poll --long-poll-mu=8.5 --long-poll-sigma=1.2 --long-poll-cap=31000 --long-poll-kill-pct=20" \
  docker compose up
