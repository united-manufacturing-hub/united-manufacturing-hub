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

# Tells the staging and production Management Console about a new umh-core
# version. A vX.Y.Z-pre.N version goes to the nightly channel. A stable version
# goes to the stable channel: it promotes the current nightly if that nightly is
# a pre-release of the same version, and is a new stable entry otherwise.
#
# Enterprise versions are not set here; they are set directly in the database.
#
# Env:  VERSION                 tag of the version, e.g. v0.45.3 (required)
#       UMH_MC_WEBHOOK_SECRET   webhook secret (required)

set -euo pipefail

: "${VERSION:?VERSION is required}"
: "${UMH_MC_WEBHOOK_SECRET:?UMH_MC_WEBHOOK_SECRET is required}"

post_version() {
  curl --fail --silent --show-error --max-time 30 --retry 3 -X POST "$1/api/v2/webhook/versions/umh-core/update" \
    -H "X-Webhook-Secret: $UMH_MC_WEBHOOK_SECRET" \
    -H "Content-Type: application/json" \
    -d "$2"
}

notify_mc() {
  local base_url=$1 payload

  echo "Notifying $base_url of new version: $VERSION"

  if [[ "$VERSION" == *-pre.* ]]; then
    payload=$(jq -nc --arg v "$VERSION" '{channel: "nightly", version: $v}')
  else
    local nightly_version
    nightly_version=$(curl --fail --silent --show-error --max-time 10 -X GET "$base_url/api/v2/versions/umh-core/nightly" | jq -r '.version')
    # v0.45.3-pre.1 -> v0.45.3
    if [[ "$VERSION" == "${nightly_version%-pre.*}" ]]; then
      payload=$(jq -nc --arg v "$VERSION" '{channel: "stable", version: $v, isPromotion: true, promotedFrom: "nightly"}')
    else
      payload=$(jq -nc --arg v "$VERSION" '{channel: "stable", version: $v}')
    fi
  fi

  post_version "$base_url" "$payload"
  echo "Successfully notified $base_url"
}

notify_mc "https://staging.management.umh.app"
notify_mc "https://management.umh.app"
