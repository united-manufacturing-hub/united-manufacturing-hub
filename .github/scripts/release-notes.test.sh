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

# Tests for release-notes.sh.
#
# Runnable locally:
#   .github/scripts/release-notes.test.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FAILED=0
CHANGELOG_FILE="$(mktemp)"
trap 'rm -f "$CHANGELOG_FILE"' EXIT
export CHANGELOG_FILE

cat > "$CHANGELOG_FILE" <<'MD'
# Changelog

## [0.45.5]

## [0.45.4]

### Fixes

- Four.

## [0.45.3]

### New Features

- Three.

## [0.45.2]

### Fixes

- Two.
MD

notes() {
  VERSION="$1" RELEASE_TAGS="$2" "$SCRIPT_DIR/release-notes.sh"
}

check() {
  local name="$1" want="$2" got="$3"
  if [ "$want" = "$got" ]; then
    echo "ok   $name"
  else
    printf 'FAIL %s:\n--- want\n%s\n--- got\n%s\n' "$name" "$want" "$got"
    FAILED=1
  fi
}

check "a release without superseded versions has its own section" \
  "$(printf '\n### Fixes\n\n- Two.')" \
  "$(notes 0.45.2 "v0.45.1 v0.45.2")"

check "a release adds the sections of the versions it superseded" \
  "$(printf '\n### Fixes\n\n- Four.\n\n### Also in this release: v0.45.3\n\nv0.45.3 was not released on its own. Its changes ship with this release.\n\n#### New Features\n\n- Three.')" \
  "$(notes 0.45.4 "v0.45.2
v0.45.4")"

check "a tagged version below is not added" \
  "$(printf '\n### Fixes\n\n- Four.')" \
  "$(notes 0.45.4 "v0.45.2 v0.45.3 v0.45.4")"

check "a blank section prints nothing" "" "$(notes 0.45.5 "v0.45.2")"
check "a missing section prints nothing" "" "$(notes 0.45.9 "v0.45.2")"

exit "$FAILED"
