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

# Prints the newest released umh-core version: the newest stable vX.Y.Z tag on
# the remote "origin", e.g. v0.45.2. Prints nothing if there is none.
#
# Runnable locally:
#   .github/scripts/latest-release-tag.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
tags="$("$SCRIPT_DIR/release-tags.sh")"
printf '%s\n' "$tags" | tail -1
