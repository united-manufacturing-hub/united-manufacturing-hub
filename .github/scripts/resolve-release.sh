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

# Finds out what a push to staging releases, for release.yml.
#
# A release PR (prepare-release.yml) moves the fragments in
# umh-core/.changelog/unreleased/ into a new umh-core/.changelog/<version>.md.
# Its merge commit M is the one that adds that file. The release PR is one
# commit on top of the staging commit S it was generated from, so S is M^2^1.
# The release ships S. The changelog describes exactly the code at S. PRs that
# merged into staging after S keep their fragments and ship with the next
# release.
#
# A release ships the version file its release PR adds, which must be the
# newest one. Every other version file that is newer than the newest stable
# vX.Y.Z tag belongs to a release that failed. Such a version is superseded: it
# gets no tag, image or GitHub Release of its own, and its changes ship with
# the released version.
#
# The newest tag marks what is released, so it must have its GitHub Release. A
# tag without one means that release's run failed after tagging, and the script
# fails until that run is completed.
#
# Env:  BEFORE, AFTER  commit range of a push to staging. The script looks for a
#                      release PR merge in that range.
#       DETECT_ONLY    if true, only finds M and outputs release and merge; the
#                      release is not validated
#       HAS_RELEASE    command that tells whether the GitHub Release of a tag
#                      exists (default: github-release-exists.sh, which needs
#                      GH_TOKEN and GITHUB_REPOSITORY)
# Outputs (written to $GITHUB_OUTPUT, or stdout when unset):
#       release   true if the push contains a release PR merge, otherwise false
#       merge     M
#       base      S
#       version   the version to release (bare semver, e.g. "0.45.3")
#
# Works in a shallow, blobless clone (actions/checkout with filter: blob:none
# and the default fetch-depth). The full history is too big to fetch, so the
# script fetches only as much history as it needs, in steps of DEEPEN_STEP
# commits and at most MAX_DEPTH commits. It reads the tags from the remote
# "origin".
# Runnable locally:
#   BEFORE=<sha> AFTER=<sha> .github/scripts/resolve-release.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHANGES_DIR="umh-core/.changelog"
CHANGELOG="umh-core/CHANGELOG.md"
VERSION_FILE_RE='^umh-core/\.changelog/[0-9]+\.[0-9]+\.[0-9]+\.md$'
FRAGMENT_RE='^umh-core/\.changelog/unreleased/[^/]+\.yaml$'
OUTPUT="${GITHUB_OUTPUT:-/dev/stdout}"
DEEPEN_STEP=50
MAX_DEPTH=1000
HEAD_SHA="$(git rev-parse HEAD)"
HAS_RELEASE="${HAS_RELEASE:-$SCRIPT_DIR/github-release-exists.sh}"

fail() {
  echo "::error::$1" >&2
  exit 1
}

# Runs the command until it succeeds. In a shallow clone, each failure fetches
# DEEPEN_STEP more commits of the history of HEAD. Fails once the clone is
# complete or MAX_DEPTH commits are fetched.
deepen_until() {
  local depth=0
  until "$@" >/dev/null 2>&1; do
    if [ "$(git rev-parse --is-shallow-repository)" != true ] || [ "$depth" -ge "$MAX_DEPTH" ]; then
      return 1
    fi
    git fetch --quiet --no-tags --deepen="$DEEPEN_STEP" origin "$HEAD_SHA"
    depth=$((depth + DEEPEN_STEP))
  done
}

# Prints the full SHA of the commit in the fetched history of HEAD whose SHA
# starts with $1; fails if there is none. Looking a missing commit up by name
# instead would make the partial clone fetch it with its entire history.
find_in_history() {
  git rev-list HEAD | awk -v sha="$1" 'index($0, sha) == 1 && !found { found = $0 } END { if (!found) exit 1; print found }'
}

# Prints the version files that commit $1 adds to its first parent.
added_version_files() {
  git diff --no-renames --diff-filter=A --name-only "$1^1" "$1" -- "$CHANGES_DIR" \
    | grep -E "$VERSION_FILE_RE" || true
}

version_of() {
  local name="${1#"$CHANGES_DIR"/}"
  printf '%s\n' "${name%.md}"
}

# True if version $1 sorts after version $2. Both are stable X.Y.Z versions,
# which `sort -V` orders correctly (0.45.9 before 0.45.10).
newer_than() {
  [ "$1" != "$2" ] && [ "$(printf '%s\n%s\n' "$1" "$2" | sort -V | tail -1)" = "$1" ]
}

# --- Find the release PR merge M -----------------------------------------------

: "${BEFORE:?BEFORE is required}"
: "${AFTER:?AFTER is required}"
if [ -z "${BEFORE//0/}" ]; then
  echo "The push created the branch; there is no commit range to search." >&2
  echo "release=false" >> "$OUTPUT"
  exit 0
fi
deepen_until find_in_history "$BEFORE" \
  || fail "${BEFORE} is not among the last ${MAX_DEPTH} commits of ${AFTER}."
# Several PRs can land in one push. The newest release PR merge in the range
# wins. An older one in the same push is superseded by it.
M=""
for commit in $(git rev-list --first-parent "${BEFORE}..${AFTER}"); do
  [ -n "$(added_version_files "$commit")" ] || continue
  # S is only known for a merge commit. A squash merge would release the
  # staging tip including PRs that are not in the changelog.
  [ -n "$(git rev-parse --verify --quiet "${commit}^2")" ] \
    || fail "${commit} adds a version file but is not a merge commit. Merge the release PR with a merge commit."
  M="$commit"
  break
done
if [ -z "$M" ]; then
  echo "No release PR merged in ${BEFORE}..${AFTER}." >&2
  echo "release=false" >> "$OUTPUT"
  exit 0
fi
if [ "${DETECT_ONLY:-false}" = true ]; then
  echo "Release PR merge: ${M}" >&2
  {
    echo "release=true"
    echo "merge=${M}"
  } >> "$OUTPUT"
  exit 0
fi

deepen_until git rev-parse --verify --quiet "${M}^2^1" \
  || fail "The release PR's commit is not among the last ${MAX_DEPTH} commits of staging."
S="$(git rev-parse "${M}^2^1")"
# In a shallow clone, is-ancestor fails until the history from M^1 down to S is
# fetched; it never succeeds for a commit that is not on staging.
deepen_until git merge-base --is-ancestor "$S" "${M}^1" \
  || fail "The release PR's base ${S} is not on staging, or not among its last ${MAX_DEPTH} commits. A release PR is one commit on top of staging."
echo "Release PR merge: ${M}" >&2
echo "Release base:     ${S}" >&2

# --- Check that the release PR only batched the changelog -------------------------
# The release PR may only add the version file, delete fragments and regenerate
# CHANGELOG.md. Together with the check that no fragment is left, this makes the
# released changelog match the code at S exactly.

while IFS=$'\t' read -r status path; do
  case "$status" in
    D) [[ "$path" =~ $FRAGMENT_RE ]] && continue ;;
    A) [[ "$path" =~ $VERSION_FILE_RE ]] && continue ;;
    M) [ "$path" = "$CHANGELOG" ] && continue ;;
  esac
  fail "The release PR changes ${path} (${status}). A release PR may only move the fragments of its base into a version file and regenerate ${CHANGELOG}."
done < <(git diff --no-renames --name-status "$S" "${M}^2")

if git ls-tree --name-only "${M}^2" -- "${CHANGES_DIR}/unreleased/" | grep -c '\.yaml$' >/dev/null; then
  fail "The release PR leaves fragments in ${CHANGES_DIR}/unreleased/. Its changelog does not match its base commit ${S}."
fi

# --- Find the version to release --------------------------------------------------

LATEST_TAG_VERSION="$("$SCRIPT_DIR/latest-release-tag.sh")"
LATEST_TAG_VERSION="${LATEST_TAG_VERSION#v}"
# The newest tag marks what is released. Without one, there is no way to tell a
# failed release from one that shipped.
[ -n "$LATEST_TAG_VERSION" ] \
  || fail "No vX.Y.Z tag exists. Tag the last released version first."
echo "Newest release tag: v${LATEST_TAG_VERSION}" >&2

# The tag job creates the tag before the GitHub Release. If the release is
# missing, that run failed in between, and a re-run of its failed jobs creates
# it. Releasing past it would treat the version as shipped.
has_release=0
"$HAS_RELEASE" "v${LATEST_TAG_VERSION}" || has_release=$?
case "$has_release" in
  0) ;;
  3) fail "v${LATEST_TAG_VERSION} has only a draft GitHub Release. Publish or delete the draft, then re-run the failed jobs of this run. See umh-core/RELEASING.md." ;;
  1) fail "v${LATEST_TAG_VERSION} is tagged but has no GitHub Release: its release run failed. Re-run the failed jobs of the Release run of v${LATEST_TAG_VERSION} first, then the failed jobs of this run. See umh-core/RELEASING.md." ;;
  *) fail "Could not check whether v${LATEST_TAG_VERSION} has a GitHub Release." ;;
esac

VERSIONS=()
while IFS= read -r file; do
  version="$(version_of "$file")"
  if newer_than "$version" "$LATEST_TAG_VERSION"; then
    VERSIONS+=("$version")
  fi
done < <(git ls-tree --name-only "$M" -- "${CHANGES_DIR}/" | grep -E "$VERSION_FILE_RE" | sort -V)

ADDED="$(added_version_files "$M")"
[ "$(printf '%s\n' "$ADDED" | wc -l)" -eq 1 ] \
  || fail "The release PR adds more than one version file: $(printf '%s ' $ADDED)"
VERSION="$(version_of "$ADDED")"
newer_than "$VERSION" "$LATEST_TAG_VERSION" \
  || fail "The release PR adds ${VERSION}, which is not newer than v${LATEST_TAG_VERSION}."
NEWEST="${VERSIONS[${#VERSIONS[@]}-1]}"
[ "$NEWEST" = "$VERSION" ] \
  || fail "The release PR adds ${VERSION}, but ${NEWEST} is newer and has no tag."
if [ "${#VERSIONS[@]}" -gt 1 ]; then
  echo "Superseded by ${VERSION}: ${VERSIONS[*]:0:${#VERSIONS[@]}-1}" >&2
fi

# The version needs a non-empty CHANGELOG.md section: the Linear sync publishes
# it with build-release-notes.sh, and fails after the release is published
# otherwise.
CHANGELOG_AT_M="$(mktemp)"
NOTES="$(mktemp)"
trap 'rm -f "$CHANGELOG_AT_M" "$NOTES"' EXIT
git show "${M}:${CHANGELOG}" > "$CHANGELOG_AT_M"
VERSION="$VERSION" CHANGELOG_FILE="$CHANGELOG_AT_M" DRY_RUN=false \
  "$SCRIPT_DIR/build-release-notes.sh" "$NOTES" >&2

echo "Version: ${VERSION}" >&2
{
  echo "release=true"
  echo "merge=${M}"
  echo "base=${S}"
  echo "version=${VERSION}"
} >> "$OUTPUT"
