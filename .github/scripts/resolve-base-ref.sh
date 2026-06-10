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

# Prints the commit-scan base for $CURRENT_TAG to stdout (empty = let the CLI
# pick its automatic baseline). Diagnostics go to stderr.
#
# The CLI's automatic baseline (and Linear's stored baseline) can break across
# releases, so we resolve from the immediately-preceding stable semver tag: if
# it is a direct ancestor of CURRENT_TAG it is the exact window start; otherwise
# its first parent sits on the shared line. A BASE_REF override short-circuits
# all of this.
#
# Tags are filtered to stable `vX.Y.Z` (pre-releases like v1.2.3-pre.4 excluded)
# and ordered with `sort -V`: semver is NOT lexically sortable (v0.44.10 sorts
# before v0.44.9 under a plain/refname sort), so version sort is mandatory here.
#
# Env:  CURRENT_TAG  release tag being synced, e.g. v0.44.21 (required)
#       BASE_REF     explicit override (tag or SHA); printed as-is if set
#
# Runnable locally:
#   CURRENT_TAG=v0.44.21 .github/scripts/resolve-base-ref.sh

set -euo pipefail

: "${CURRENT_TAG:?CURRENT_TAG is required}"

if [ -n "${BASE_REF:-}" ]; then
  printf '%s\n' "$BASE_REF"
  exit 0
fi

# Guard: CURRENT_TAG must itself be a stable vX.Y.Z tag. Otherwise the awk below
# yields an empty PREV_TAG that is indistinguishable from a true first release,
# and we would silently defer to the unreliable auto-baseline. The release:
# event gate enforces the 'v' prefix, but a workflow_dispatch can pass any tag.
if ! printf '%s\n' "$CURRENT_TAG" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
  echo "::error::${CURRENT_TAG} is not a stable vX.Y.Z tag; refusing to guess a scan base." >&2
  exit 1
fi

PREV_TAG="$(git tag -l 'v*' \
  | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' \
  | sort -V \
  | awk -v cur="$CURRENT_TAG" '$0==cur{print prev; exit} {prev=$0}')"

# Distinguish "not an ancestor" (exit 1 — a normal answer) from a real git
# failure (exit >1: bad ref, missing object, unfetched history). The latter
# must fail loud: silently falling back to the CLI auto-baseline is the exact
# unreliable behavior this resolver exists to prevent. `|| rc=$?` keeps set -e
# from aborting on the expected non-zero.
is_ancestor() {
  local rc=0
  git merge-base --is-ancestor "$1" "$CURRENT_TAG" 2>/dev/null || rc=$?
  if [ "$rc" -gt 1 ]; then
    echo "::error::git merge-base failed (exit ${rc}) for ${1} vs ${CURRENT_TAG}." >&2
    exit "$rc"
  fi
  return "$rc"
}

if [ -z "$PREV_TAG" ]; then
  # Legitimate first-release case: no predecessor, defer to the CLI baseline.
  echo "::warning::No stable vX.Y.Z tag precedes ${CURRENT_TAG}; using the CLI automatic baseline." >&2
elif is_ancestor "$PREV_TAG"; then
  # Normal case: the previous release tag is a direct ancestor, so it is the
  # exact start of this release's window. A rev-parse failure here is a real
  # error and aborts (set -e) rather than shipping a wrong base.
  BASE_SHA="$(git rev-parse "$PREV_TAG")"
  printf '%s\n' "$BASE_SHA"
  echo "Scan base: ${PREV_TAG} (${BASE_SHA})" >&2
elif is_ancestor "${PREV_TAG}^1"; then
  # The previous tag is a merge commit off this tag's line (process
  # inconsistency); its first parent sits on the shared line.
  BASE_SHA="$(git rev-parse "${PREV_TAG}^1")"
  printf '%s\n' "$BASE_SHA"
  echo "Scan base: ${PREV_TAG}^1 (${BASE_SHA}) [previous tag is not a direct ancestor]" >&2
else
  echo "::warning::Neither ${PREV_TAG} nor its first parent is an ancestor of ${CURRENT_TAG}; using the CLI automatic baseline." >&2
fi
