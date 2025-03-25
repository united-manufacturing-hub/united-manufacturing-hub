#!/usr/bin/env bash

user_cwd=$(pwd)

function cleanup {
    cd "$user_cwd" || exit 1
}

trap cleanup EXIT

echo "Moving to the script's directory"
cd "$(dirname "$0")" || exit 1

# Install quicktype to generate Go code from JSON schemas
if ! command -v quicktype &> /dev/null
then
  echo "Installing quicktype"
  npm install -g quicktype || exit 1
fi

# Install jq to format and validate JSON schemas
if ! command -v jq &> /dev/null
then
  echo "Installing jq"
  sudo apt-get install -y jq || exit 1
fi

# Find all *.schema.json files recursively, excluding those that begin with common
schema_files=$(find . -type f -name "*.schema.json" ! -name "common*.schema.json")

# Format and validate each JSON schema file
for schema_file in $schema_files; do
  echo "Formatting and validating $schema_file"
  jq . "$schema_file" > tmp.$$.json && mv tmp.$$.json "$schema_file"
  if [ $? -ne 0 ]; then
      echo "Failed to format or validate $schema_file"
      exit 1
  fi
done

git_root=$(git rev-parse --show-toplevel) || exit 1
echo "Git root: $git_root"

generated_dir="$git_root/umh-core/pkg/generated"
mkdir -p "$generated_dir"

# Copyright header to prepend to each generated file
copyright_header='// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.'

echo "Generating Go files"
for schema_file in $schema_files; do
    echo "Processing $schema_file"
    base_name=$(basename "$schema_file" .schema.json)
    temp_file=$(mktemp)

    quicktype \
        --src "$schema_file" \
        --src-lang schema \
        --lang go \
        --package generated \
        --top-level "${base_name}" \
        --out "$temp_file" || exit 1

    output_file="$generated_dir/${base_name}.go"
    { echo "$copyright_header"; echo ""; cat "$temp_file"; } > "$output_file"
    rm "$temp_file"
done