#!/bin/sh
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