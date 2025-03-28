#!/bin/sh

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

while true; do
  echo "NMAP_SCAN_START"
  echo "NMAP_TIMESTAMP: $(date -Iseconds)"
  SCAN_START=$(date +%s.%N)
  echo "NMAP_COMMAND: nmap -n -Pn -p 443 umh.app -v"
  nmap -n -Pn -p 443 umh.app -v
  EXIT_CODE=$?
  SCAN_END=$(date +%s.%N)
  SCAN_DURATION=$(echo "$SCAN_END - $SCAN_START" | bc)
  echo "NMAP_EXIT_CODE: $EXIT_CODE"
  echo "NMAP_DURATION: $SCAN_DURATION"
  echo "NMAP_SCAN_END"
  sleep 1
done