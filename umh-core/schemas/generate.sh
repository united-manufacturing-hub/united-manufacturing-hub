#!/usr/bin/env bash

user_cwd=$(pwd)

# On exit, change back to the user's original working directory
function cleanup {
    cd "$user_cwd" || exit 1
}

# Set up a trap to call the cleanup function on script exit or error
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

# Format and validate each JSON schema file using jq
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

echo "Generating Go files"
for schema_file in $schema_files; do
    base_name=$(basename "$schema_file" .schema.json)
    echo "Processing $schema_file"

    quicktype \
        --src "$schema_file" \
        --src-lang schema \
        --lang go \
        --package generated \
        --top-level "${base_name}" \
        --out "$generated_dir/${base_name}.go" || exit 1
done