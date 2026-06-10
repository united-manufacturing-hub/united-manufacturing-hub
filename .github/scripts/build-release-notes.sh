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

# Writes the Linear release notes for $VERSION into the file named by $1.
#
# The notes are the `## [VERSION]` section of the CHANGELOG (image-only lines
# stripped; the image-rich changelog lives at umh.app/insights/changelog) then a
# footer linking there. An empty/missing section yields just the footer plus a
# workflow warning on dry-runs, or a hard failure on a real sync.
#
# umh-core's CHANGELOG.md uses bracketed semver headings (`## [0.44.21]`), so
# VERSION here is the BARE semver (no leading 'v'); the v-prefixed tag is only
# used for the Linear release version, not the changelog lookup.
#
# Env:  VERSION         bare semver, e.g. 0.44.21 (required)
#       CHANGELOG_FILE   source changelog (default: umh-core/CHANGELOG.md)
#       DRY_RUN          'true' downgrades empty-notes failure to a warning
# Args: $1              output notes path (required)
#
# Runnable locally:
#   VERSION=0.44.21 .github/scripts/build-release-notes.sh /tmp/notes.md

set -euo pipefail

: "${VERSION:?VERSION is required}"
CHANGELOG_FILE="${CHANGELOG_FILE:-umh-core/CHANGELOG.md}"
OUT="${1:?output notes path is required (arg 1)}"

# Extract the `## [VERSION]` section, stopping at the next `## [` heading. The
# `## Unreleased` heading sits above all versioned entries, so it is never
# crossed mid-extraction. Then drop image-only lines.
awk -v v="$VERSION" '
  $0 == "## [" v "]" {found=1; next}
  /^## \[/ {if (found) exit}
  found {print}
' "$CHANGELOG_FILE" \
  | sed -E '/^[[:space:]]*!\[[^]]*\]\([^)]*\)[[:space:]]*$/d' > "$OUT"

# Test for non-whitespace content, not just non-zero size: an image-only
# section leaves blank lines after the image-strip above, which a `-s` test
# would wrongly accept and ship as footer-only notes.
#
# On a real sync, footer-only notes almost always mean a mistyped/forgotten
# `## [x.y.z]` heading (hand-edited at release time), so fail loud rather than
# silently publishing an empty Linear description. Dry-runs stay green so
# odd/historical tags can still be exercised.
if ! grep -q '[^[:space:]]' "$OUT"; then
  if [ "${DRY_RUN:-false}" = "true" ]; then
    echo "::warning::No ${CHANGELOG_FILE} section found for [${VERSION}]; release notes will contain only the footer."
    : > "$OUT"
  else
    echo "::error::No ${CHANGELOG_FILE} section found for [${VERSION}]; refusing to publish footer-only release notes. Check the '## [${VERSION}]' heading in ${CHANGELOG_FILE}."
    exit 1
  fi
fi

{
  echo ""
  echo "---"
  echo "Full changelog: https://umh.app/insights/changelog"
} >> "$OUT"
