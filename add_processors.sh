#!/bin/bash

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

set -euo pipefail

# Check if number of processors is provided
if [ $# -eq 0 ]; then
    echo "Usage: $0 <number_of_processors> [config_file]"
    echo "Example: $0 3 umh-core/data/config.yaml"
    exit 1
fi

NUM_PROCESSORS="$1"
CONFIG_FILE="${2:-umh-core/data/config.yaml}"

# Validate number is positive integer
if ! [[ "$NUM_PROCESSORS" =~ ^[1-9][0-9]*$ ]]; then
    echo "Error: Number of processors must be a positive integer"
    exit 1
fi

# Check if config file exists
if [ ! -f "$CONFIG_FILE" ]; then
    echo "Error: Config file '$CONFIG_FILE' not found"
    exit 1
fi

# Create backup
cp "$CONFIG_FILE" "${CONFIG_FILE}.backup"
echo "Created backup: ${CONFIG_FILE}.backup"

# Find the line number where tag_processor starts
TAG_PROCESSOR_START=$(grep -n "\- tag_processor:" "$CONFIG_FILE" | cut -d: -f1)

if [ -z "$TAG_PROCESSOR_START" ]; then
    echo "Error: tag_processor not found in config file"
    exit 1
fi

# Get the indentation level of the tag_processor line (number of spaces before the dash)
TAG_PROCESSOR_INDENT=$(sed -n "${TAG_PROCESSOR_START}p" "$CONFIG_FILE" | sed 's/\(^ *\).*/\1/' | wc -c)
TAG_PROCESSOR_INDENT=$((TAG_PROCESSOR_INDENT - 1))  # subtract 1 because wc -c counts the newline

# Find the insertion point - insert before the variables section
INSERTION_LINE=$(awk -v start="$TAG_PROCESSOR_START" '/^ *variables:/ && NR > start {print NR - 1; exit}' "$CONFIG_FILE")

if [ -z "$INSERTION_LINE" ]; then
    echo "Error: Could not find insertion point after tag_processor"
    exit 1
fi



# Generate the processors to add with proper indentation
INDENT_SPACES=$(printf "%*s" "$TAG_PROCESSOR_INDENT" "")
PROCESSORS_TO_ADD=""
for i in $(seq 1 "$NUM_PROCESSORS"); do
    PROCESSORS_TO_ADD="${PROCESSORS_TO_ADD}${INDENT_SPACES}- bloblang: root = content()\n"
done

# Create temporary file with the new content
TEMP_FILE=$(mktemp)

# Insert the processors at the correct line
head -n "$INSERTION_LINE" "$CONFIG_FILE" > "$TEMP_FILE"
echo -e "$PROCESSORS_TO_ADD" | sed '$d' >> "$TEMP_FILE"
tail -n +$((INSERTION_LINE + 1)) "$CONFIG_FILE" >> "$TEMP_FILE"

# Replace the original file
mv "$TEMP_FILE" "$CONFIG_FILE"

echo "Successfully added $NUM_PROCESSORS processor(s) to $CONFIG_FILE"
echo "Added processors with 'bloblang: root = content()' configuration"