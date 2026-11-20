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

# Tests for resolve-base-ref.sh, in a throwaway git repository.
#
# Runnable locally:
#   .github/scripts/resolve-base-ref.test.sh

set -euo pipefail

SCRIPT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/resolve-base-ref.sh"
export GIT_AUTHOR_NAME=test GIT_AUTHOR_EMAIL=test@example.com
export GIT_COMMITTER_NAME=test GIT_COMMITTER_EMAIL=test@example.com
FAILED=0
REPO="$(mktemp -d)"
trap 'rm -rf "$REPO"' EXIT

check() {
  local name="$1" want="$2" got="$3"
  if [ "$want" = "$got" ]; then
    echo "ok   $name"
  else
    echo "FAIL $name: want '$want', got '$got'"
    FAILED=1
  fi
}

base_of() {
  CURRENT_TAG="$1" "$SCRIPT" 2>/dev/null
}

cd "$REPO"
git init -q -b staging
git commit -q --allow-empty -m one
git tag v0.45.1
FIRST="$(git rev-parse HEAD)"
git commit -q --allow-empty -m two
git tag v0.45.2
SECOND="$(git rev-parse HEAD)"
git tag v0.45.3-pre.1
# Two tags point at the same commit, as v0.25.0 and v0.26.0 do.
git commit -q --allow-empty -m three
git tag v0.45.3
git tag v0.45.4

check "a release scans from the previous tag" "$FIRST" "$(base_of v0.45.2)"
check "the first tag on a shared commit scans from the previous release" "$SECOND" "$(base_of v0.45.3)"
check "a later tag on a shared commit scans from the previous release" "$SECOND" "$(base_of v0.45.4)"
check "the first release defers to the CLI baseline" "" "$(base_of v0.45.1)"

for n in $(seq 5 10); do
  git commit -q --allow-empty -m "release $n"
  git tag "v0.45.$n"
done
NINTH="$(git rev-parse v0.45.9)"
check "v0.45.10 scans from v0.45.9" "$NINTH" "$(base_of v0.45.10)"

# Two annotated tags on one commit.
git commit -q --allow-empty -m "release 11"
git tag -a v0.45.11 -m v0.45.11
git tag -a v0.45.12 -m v0.45.12
TENTH="$(git rev-parse v0.45.10)"
check "an annotated tag on a shared commit scans from the previous release" "$TENTH" "$(base_of v0.45.12)"

exit "$FAILED"
