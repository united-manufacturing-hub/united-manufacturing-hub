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

# Prints the version of the next umh-core release, for prepare-release.yml.
# It is the newest umh-core/.changelog/X.Y.Z.md with the patch number bumped
# (0.45.2 -> 0.45.3), or with the minor number bumped if MINOR_RELEASE is true
# (0.45.2 -> 0.46.0). The newest version file is the newest release, or a failed
# release that the next one supersedes.
#
# Env:  MINOR_RELEASE  true for a minor release (default: false)
# Args: $1             directory of the version files (default: umh-core/.changelog)
#
# Runnable locally:
#   .github/scripts/next-release-version.sh

set -euo pipefail

CHANGES_DIR="${1:-umh-core/.changelog}"

latest="$(find "$CHANGES_DIR" -maxdepth 1 -name '*.md' -exec basename {} .md \; \
  | { grep -E '^[0-9]+\.[0-9]+\.[0-9]+$' || true; } | sort -V | tail -1)"
if [ -z "$latest" ]; then
  echo "::error::No X.Y.Z.md version file in ${CHANGES_DIR}." >&2
  exit 1
fi

IFS=. read -r major minor patch <<<"$latest"
if [ "${MINOR_RELEASE:-false}" = true ]; then
  echo "${major}.$((minor + 1)).0"
else
  echo "${major}.${minor}.$((patch + 1))"
fi
