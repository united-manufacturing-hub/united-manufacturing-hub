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

# Runs the cpu-host scenario in a Linux container, because the CPU monitor
# reads cgroup v2 files that macOS and Windows machines do not publish.
# Builds the scenario runner for Linux on this machine, then runs it in a
# stock Alpine image with no toolchain and no repository mounted.
#
# Environment variables:
#   CPUS       CPU quota for the container, e.g. CPUS=2. Below umh-core's
#              own minimum the reserves leave numbers describing no machine
#              we support, so prefer 2 over 0.5. Unset means no
#              quota, which is the other world worth watching: no throttling
#              signal, only host load.
#   DURATION   How long the monitor runs after its first reading. Default 0,
#              which runs until Ctrl+C, so you can put load on the container
#              and watch the readings answer it for as long as you like.
#   LOG_LEVEL  Runner log level (default debug, which carries every completed
#              poll on the cpu_reading line; info carries the verdict and its
#              message whenever the worker's state changes).
#
# Any argument is passed to docker run verbatim, so docker's own options
# work without this script naming them (e.g. --name, to make the container
# easy to exec into while it runs).
#
# The container gets stress-ng, so you can load it from a second terminal
# without installing anything there:
#
#     docker exec cpu-host stress-ng --cpu 8 --timeout 60

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Build outside the repository: go build on a main package writes its
# output into the working directory otherwise, which would drop a binary
# into the checkout.
BIN="$(mktemp /tmp/cpu-host-runner.XXXXXX)"
trap 'rm -f "$BIN"' EXIT

cd "$ROOT"
CGO_ENABLED=0 GOOS=linux GOARCH="$(go env GOARCH)" go build -o "$BIN" ./pkg/fsmv2/cmd/runner

docker_args=(--rm -v "$BIN":/runner:ro)
if [ -n "${CPUS:-}" ]; then
  docker_args+=(--cpus="$CPUS")
fi
docker_args+=("$@")
docker_args+=(
  alpine:latest
  # stress-ng comes from Alpine's community repository, which the official
  # image already has enabled. Installing it here rather than in a Dockerfile
  # keeps this a stock image with one mounted binary. If the install fails —
  # no network, most likely — say so and still run the monitor: only the
  # in-container load experiment needs stress-ng.
  sh -c 'apk add --no-cache stress-ng >/dev/null 2>&1 ||
           echo "cpu-host: stress-ng install failed; the monitor still runs, docker exec stress-ng will not"
         exec /runner -scenario cpu-host -duration "$1" -log-level "$2"' \
    cpu-host "${DURATION:-0}" "${LOG_LEVEL:-debug}"
)

docker run "${docker_args[@]}"
