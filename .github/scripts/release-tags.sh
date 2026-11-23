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

# Prints every stable umh-core release tag (vX.Y.Z) on the remote "origin",
# oldest first. Pre-release tags (v0.45.3-pre.1) are nightlies and do not count.
# `sort -V` orders the versions correctly (v0.45.9 before v0.45.10).
#
# Runnable locally:
#   .github/scripts/release-tags.sh

set -euo pipefail

git ls-remote --tags --refs origin 'v*' \
  | sed -nE 's#^.*refs/tags/(v[0-9]+\.[0-9]+\.[0-9]+)$#\1#p' \
  | sort -V
