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

# Prints the release notes of $VERSION from CHANGELOG.md, for the GitHub
# Release, changelog.umh.app and Linear.
#
# The notes are the `## [VERSION]` section, then the section of each version
# that VERSION superseded: the untagged versions below it, down to the next
# tagged version. A superseded version belongs to a release that failed; it has
# no tag and no release of its own, and its changes ship with VERSION. Its
# section gets a `### Also in this release: vX.Y.Z` heading, and its own
# headings move one level down.
#
# Prints nothing if the `## [VERSION]` section is missing or blank.
#
# Env:  VERSION         bare semver, e.g. 0.45.4 (required)
#       CHANGELOG_FILE  source changelog (default: umh-core/CHANGELOG.md)
#       RELEASE_TAGS    the stable release tags, separated by whitespace
#                       (default: the output of release-tags.sh)
#
# Runnable locally:
#   VERSION=0.45.2 .github/scripts/release-notes.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
: "${VERSION:?VERSION is required}"
CHANGELOG_FILE="${CHANGELOG_FILE:-umh-core/CHANGELOG.md}"
if [ -z "${RELEASE_TAGS+set}" ]; then
  RELEASE_TAGS="$("$SCRIPT_DIR/release-tags.sh")"
fi

# The notes are collected in `out` and printed only if the version's own
# section has content.
awk -v v="$VERSION" -v tags="$(printf '%s' "$RELEASE_TAGS" | tr '\n' ' ')" '
  BEGIN { n = split(tags, t, " "); for (i = 1; i <= n; i++) tagged[t[i]] = 1 }
  /^## \[/ {
    h = $0; sub(/^## \[/, "", h); sub(/\].*$/, "", h)
    if (!found) { found = (h == v); next }
    if (("v" h) in tagged) exit
    if (last != "") out = out "\n"
    out = out "### Also in this release: v" h "\n\nv" h " was not released on its own. Its changes ship with this release.\n"
    superseded = 1
    next
  }
  !found { next }
  superseded && /^#/ { $0 = "#" $0 }
  !superseded && /[^[:space:]]/ { has_content = 1 }
  { out = out $0 "\n"; last = $0 }
  END { if (has_content) printf "%s", out }
' "$CHANGELOG_FILE"
