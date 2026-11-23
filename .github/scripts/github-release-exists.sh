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

# Tells whether the GitHub Release for tag $1 exists in $GITHUB_REPOSITORY.
# Exit codes:
#   0  the release is published
#   1  there is no release for the tag
#   2  the lookup failed (for example an API or auth error); a caller must
#      treat this as unknown, not as absent
#   3  there is only a draft release for the tag; it must be published or
#      deleted
#
# Env:  GH_TOKEN, GITHUB_REPOSITORY  as for the gh CLI
#
# Runnable locally:
#   GITHUB_REPOSITORY=united-manufacturing-hub/united-manufacturing-hub \
#     .github/scripts/github-release-exists.sh v0.45.2; echo $?

set -euo pipefail

: "${1:?tag is required}"
: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"

if out="$(gh api "repos/${GITHUB_REPOSITORY}/releases/tags/$1" --silent 2>&1)"; then
  exit 0
fi
if ! printf '%s\n' "$out" | grep -c 'HTTP 404' >/dev/null; then
  printf '%s\n' "$out" >&2
  exit 2
fi
# The tag lookup does not see drafts; only the list of releases does.
drafts="$(gh api --paginate "repos/${GITHUB_REPOSITORY}/releases" \
  --jq "[.[] | select(.draft and .tag_name == \"$1\")] | length")" || exit 2
if printf '%s\n' "$drafts" | grep -cv '^0$' >/dev/null; then
  echo "$1 has a draft GitHub Release." >&2
  exit 3
fi
exit 1
