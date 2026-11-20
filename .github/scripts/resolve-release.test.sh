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

# Tests for resolve-release.sh. Each case builds a throwaway git repository
# that mimics staging: PRs land as merge commits, and a release PR is one commit
# on top of its base that batches the fragments.
#
# Runnable locally:
#   .github/scripts/resolve-release.test.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
export GIT_AUTHOR_NAME=test GIT_AUTHOR_EMAIL=test@example.com
export GIT_COMMITTER_NAME=test GIT_COMMITTER_EMAIL=test@example.com
# Automatic gc and maintenance run in the background and repack objects while
# the next git command of a case reads them, which fails that command.
export GIT_CONFIG_COUNT=2
export GIT_CONFIG_KEY_0=gc.auto GIT_CONFIG_VALUE_0=0
export GIT_CONFIG_KEY_1=maintenance.auto GIT_CONFIG_VALUE_1=false
FAILED=0
ROOT="$(mktemp -d)"
trap 'rm -rf "$ROOT"' EXIT

# Stand-ins for github-release-exists.sh: the GitHub Release of the newest tag
# exists, does not exist, cannot be looked up, or is a draft.
printf '#!/bin/sh\nexit 0\n' > "$ROOT/release-present"
printf '#!/bin/sh\nexit 1\n' > "$ROOT/release-absent"
printf '#!/bin/sh\nexit 2\n' > "$ROOT/release-unknown"
printf '#!/bin/sh\nexit 3\n' > "$ROOT/release-draft"
chmod +x "$ROOT"/release-*

new_repo() {
  cd "$(mktemp -d "$ROOT/repo.XXXXXX")"
  git init -q -b staging
  mkdir -p umh-core/.changelog/unreleased
  touch umh-core/.changelog/unreleased/.gitkeep
  printf '# Changelog\n\n## [0.45.2]\n\n### Fixes\n\n- Old fix.\n' > umh-core/CHANGELOG.md
  printf '## [0.45.2]\n' > umh-core/.changelog/0.45.2.md
  git add -A
  git commit -qm init
  git tag v0.45.2
  # The script reads the release tags from origin.
  git remote add origin "$PWD"
}

# Merges branch $1 into staging the way GitHub does: a merge commit whose
# second parent is the PR head.
merge_pr() {
  git checkout -q staging
  git merge -q --no-ff --no-edit "$1"
}

# A feature PR that adds the fragment $1.
feature_pr() {
  git checkout -q -b "feature-$1" staging
  echo "body: $1" > "umh-core/.changelog/unreleased/$1.yaml"
  echo "$1" > "umh-core/code-$1.txt"
  git add -A
  git commit -qm "feature $1"
}

# A release PR on top of the current staging tip that batches every fragment
# into version $1.
release_pr() {
  git checkout -q -b "release-$1" staging
  git rm -q umh-core/.changelog/unreleased/*.yaml
  printf '## [%s]\n' "$1" > "umh-core/.changelog/$1.md"
  {
    printf '# Changelog\n\n## [%s]\n\n### Fixes\n\n- A fix.\n\n' "$1"
    tail -n +3 umh-core/CHANGELOG.md
  } > umh-core/CHANGELOG.md.new
  mv umh-core/CHANGELOG.md.new umh-core/CHANGELOG.md
  git add -A
  git commit -qm "release $1"
}

# Runs the script and stores its outputs in OUT; RC holds its exit code.
run() {
  OUT_FILE="$(mktemp "$ROOT/out.XXXXXX")"
  RC=0
  GITHUB_OUTPUT="$OUT_FILE" HAS_RELEASE="$ROOT/release-present" "$@" "$SCRIPT_DIR/resolve-release.sh" 2>/dev/null || RC=$?
  OUT="$(cat "$OUT_FILE")"
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

output() {
  printf '%s\n' "$OUT" | sed -n "s/^$1=//p"
}

# Clones the current repository the way CI checks it out: one commit deep and
# without blobs, at commit $1. Prints the clone's path.
shallow_clone() {
  local source="$PWD" clone
  clone="$(mktemp -d "$ROOT/clone.XXXXXX")"
  git -C "$source" config uploadpack.allowFilter true
  git -C "$source" config uploadpack.allowAnySHA1InWant true
  git -C "$clone" init -q
  git -C "$clone" remote add origin "file://$source"
  git -C "$clone" fetch -q --no-tags --depth=1 --filter=blob:none origin "$1"
  git -C "$clone" checkout -q FETCH_HEAD
  printf '%s\n' "$clone"
}

# --- Cases ------------------------------------------------------------------------

new_repo
feature_pr a
BEFORE="$(git rev-parse staging)"
merge_pr feature-a
run env BEFORE="$BEFORE" AFTER="$(git rev-parse staging)"
check "a push without a release PR releases nothing" "0 false" "$RC $(output release)"

new_repo
feature_pr a
merge_pr feature-a
release_pr 0.45.3
S="$(git rev-parse staging)"
feature_pr b
merge_pr feature-b
BEFORE="$(git rev-parse staging)"
merge_pr release-0.45.3
run env BEFORE="$BEFORE" AFTER="$(git rev-parse staging)"
check "a release ships its base, not the staging tip" "0 true $S" "$RC $(output release) $(output base)"
check "a release ships its version" "0.45.3 $(git rev-parse staging)" "$(output version) $(output merge)"

new_repo
feature_pr a
merge_pr feature-a
release_pr 0.45.3
merge_pr release-0.45.3
feature_pr b
merge_pr feature-b
release_pr 0.45.4
BEFORE="$(git rev-parse staging)"
merge_pr release-0.45.4
run env BEFORE="$BEFORE" AFTER="$(git rev-parse staging)"
check "a version without a tag is superseded by the next release" "0 0.45.4" "$RC $(output version)"

# Pre-release tags are nightlies; they do not mark a version as released.
new_repo
git tag v0.45.3-pre.1
feature_pr a
merge_pr feature-a
release_pr 0.45.3
BEFORE="$(git rev-parse staging)"
merge_pr release-0.45.3
run env BEFORE="$BEFORE" AFTER="$(git rev-parse staging)"
check "a pre-release tag does not count as released" "0 0.45.3" "$RC $(output version)"

new_repo
for v in 0.45.3 0.45.4 0.45.5 0.45.6 0.45.7 0.45.8 0.45.9; do
  printf '## [%s]\n' "$v" > "umh-core/.changelog/$v.md"
done
git add -A
git commit -qm "older versions"
git tag v0.45.9
feature_pr a
merge_pr feature-a
release_pr 0.45.10
BEFORE="$(git rev-parse staging)"
merge_pr release-0.45.10
run env BEFORE="$BEFORE" AFTER="$(git rev-parse staging)"
check "0.45.10 is newer than v0.45.9" "0 0.45.10" "$RC $(output version)"

new_repo
feature_pr a
merge_pr feature-a
release_pr 0.45.3
echo change > umh-core/code-a.txt
git commit -q --amend -a --no-edit
BEFORE="$(git rev-parse staging)"
merge_pr release-0.45.3
run env BEFORE="$BEFORE" AFTER="$(git rev-parse staging)"
check "a release PR that changes code fails" "1" "$RC"

new_repo
feature_pr a
merge_pr feature-a
release_pr 0.45.3
echo "body: late" > umh-core/.changelog/unreleased/late.yaml
git add -A
git commit -q --amend --no-edit
BEFORE="$(git rev-parse staging)"
merge_pr release-0.45.3
run env BEFORE="$BEFORE" AFTER="$(git rev-parse staging)"
check "a release PR that leaves a fragment fails" "1" "$RC"

new_repo
feature_pr a
merge_pr feature-a
release_pr 0.45.3
git commit -q --allow-empty -m "a second commit"
BEFORE="$(git rev-parse staging)"
merge_pr release-0.45.3
run env BEFORE="$BEFORE" AFTER="$(git rev-parse staging)"
check "a release PR with more than one commit fails" "1" "$RC"

new_repo
feature_pr a
merge_pr feature-a
release_pr 0.45.3
BEFORE="$(git rev-parse staging)"
git checkout -q staging
git merge -q --squash release-0.45.3
git commit -qm "squashed release PR"
run env BEFORE="$BEFORE" AFTER="$(git rev-parse staging)"
check "a squash-merged release PR fails" "1" "$RC"

# In CI the release PR's base is more than one deepen step (50 commits) below
# the staging tip, and the history goes further back than the script needs.
new_repo
for i in $(seq 1 200); do git commit -q --allow-empty -m "old commit $i"; done
feature_pr a
merge_pr feature-a
release_pr 0.45.3
S="$(git rev-parse staging)"
git checkout -q staging
for i in $(seq 1 120); do git commit -q --allow-empty -m "staging commit $i"; done
BEFORE="$(git rev-parse staging)"
merge_pr release-0.45.3
AFTER="$(git rev-parse staging)"
cd "$(shallow_clone "$AFTER")"
run env BEFORE="$BEFORE" AFTER="$AFTER"
check "a shallow, blobless clone resolves the same base" "0 $S" "$RC $(output base)"
check "a shallow, blobless clone finds the version" "0.45.3" "$(output version)"
check "a shallow clone stays shallow" "true" "$(git rev-parse --is-shallow-repository)"

# A PR that branched off below S merges shortly before the release PR. In the
# shallow clone its branch point is fetched before S is.
new_repo
for i in $(seq 1 200); do git commit -q --allow-empty -m "old commit $i"; done
feature_pr x
feature_pr a
merge_pr feature-a
release_pr 0.45.3
S="$(git rev-parse staging)"
git checkout -q staging
for i in $(seq 1 120); do git commit -q --allow-empty -m "staging commit $i"; done
merge_pr feature-x
BEFORE="$(git rev-parse staging)"
merge_pr release-0.45.3
AFTER="$(git rev-parse staging)"
cd "$(shallow_clone "$AFTER")"
run env BEFORE="$BEFORE" AFTER="$AFTER"
check "a PR branched off below the base does not hide the base" "0 $S" "$RC $(output base)"

new_repo
feature_pr a
merge_pr feature-a
release_pr 0.45.3
echo change > umh-core/code-a.txt
git commit -q --amend -a --no-edit
BEFORE="$(git rev-parse staging)"
merge_pr release-0.45.3
run env BEFORE="$BEFORE" AFTER="$(git rev-parse staging)" DETECT_ONLY=true
check "detect-only finds the release PR merge without validating it" "0 true $(git rev-parse staging)" "$RC $(output release) $(output merge)"

new_repo
feature_pr a
merge_pr feature-a
git checkout -q -b release-empty staging
git rm -q umh-core/.changelog/unreleased/a.yaml
printf '## [0.45.3]\n' > umh-core/.changelog/0.45.3.md
git add -A
git commit -qm "release without a CHANGELOG.md section"
BEFORE="$(git rev-parse staging)"
merge_pr release-empty
run env BEFORE="$BEFORE" AFTER="$(git rev-parse staging)"
check "a version without a CHANGELOG.md section fails" "1" "$RC"

new_repo
feature_pr a
merge_pr feature-a
release_pr 0.45.3
BEFORE="$(git rev-parse staging)"
merge_pr release-0.45.3
git tag v0.45.3 "$(git rev-parse staging^2^)"
run env BEFORE="$BEFORE" AFTER="$(git rev-parse staging)"
check "a release PR that adds a tagged version fails" "1" "$RC"

new_repo
git tag -d v0.45.2 >/dev/null
feature_pr a
merge_pr feature-a
release_pr 0.45.3
BEFORE="$(git rev-parse staging)"
merge_pr release-0.45.3
run env BEFORE="$BEFORE" AFTER="$(git rev-parse staging)"
check "a release without any release tag fails" "1" "$RC"

new_repo
feature_pr a
merge_pr feature-a
release_pr 0.45.3
BEFORE="$(git rev-parse staging)"
merge_pr release-0.45.3
run env BEFORE="$BEFORE" AFTER="$(git rev-parse staging)" HAS_RELEASE="$ROOT/release-absent"
check "a newest tag without a GitHub Release fails" "1" "$RC"
run env BEFORE="$BEFORE" AFTER="$(git rev-parse staging)" HAS_RELEASE="$ROOT/release-unknown"
check "a failed GitHub Release lookup fails" "1" "$RC"
run env BEFORE="$BEFORE" AFTER="$(git rev-parse staging)" HAS_RELEASE="$ROOT/release-draft"
check "a newest tag with only a draft GitHub Release fails" "1" "$RC"
run env BEFORE="$BEFORE" AFTER="$(git rev-parse staging)" HAS_RELEASE="$ROOT/release-absent" DETECT_ONLY=true
check "detect-only does not look up the GitHub Release" "0 true" "$RC $(output release)"

# The release PR must add the newest version file.
new_repo
printf '## [0.45.4]\n' > umh-core/.changelog/0.45.4.md
git add -A
git commit -qm "an untagged newer version"
feature_pr a
merge_pr feature-a
release_pr 0.45.3
BEFORE="$(git rev-parse staging)"
merge_pr release-0.45.3
run env BEFORE="$BEFORE" AFTER="$(git rev-parse staging)"
check "a release PR that adds an older version than an untagged one fails" "1" "$RC"

exit "$FAILED"
