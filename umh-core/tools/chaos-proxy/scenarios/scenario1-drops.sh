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
