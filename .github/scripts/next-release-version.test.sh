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

# Tests for next-release-version.sh.
#
# Runnable locally:
#   .github/scripts/next-release-version.test.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FAILED=0
ROOT="$(mktemp -d)"
trap 'rm -rf "$ROOT"' EXIT

# Creates a version file directory with the files $@ and prints its path.
# unreleased/ and its fragments are not version files.
changes_dir() {
  local dir
  dir="$(mktemp -d "$ROOT/changes.XXXXXX")"
  mkdir "$dir/unreleased"
  touch "$dir/unreleased/fragment.yaml"
  for file in "$@"; do touch "$dir/$file"; done
  printf '%s\n' "$dir"
}

# Runs the script; prints its output and exit code.
run() {
  local out rc=0
  out="$(MINOR_RELEASE="$1" "$SCRIPT_DIR/next-release-version.sh" "$2" 2>/dev/null)" || rc=$?
  printf '%s %s\n' "$rc" "$out"
}

check() {
  local name="$1" want="$2" got="$3"
  if [ "$want" = "$got" ]; then
    echo "ok   $name"
  else
    echo "FAIL $name: want '$want', got '$got'"
    FAILED=1
  fi
}

DIR="$(changes_dir 0.45.1.md 0.45.2.md README.md)"
check "a patch release bumps the patch number" "0 0.45.3" "$(run false "$DIR")"
check "a minor release bumps the minor number" "0 0.46.0" "$(run true "$DIR")"

DIR="$(changes_dir 0.45.9.md 0.45.10.md 0.45.8.md)"
check "0.45.10 is newer than 0.45.9" "0 0.45.11" "$(run false "$DIR")"

DIR="$(changes_dir 0.44.30.md 0.45.0.md)"
check "a newer minor wins over a higher patch" "0 0.45.1" "$(run false "$DIR")"

DIR="$(changes_dir README.md)"
check "no version file fails" "1 " "$(run false "$DIR")"

exit "$FAILED"
