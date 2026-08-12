#!/usr/bin/env bash
# Reads machine-readable status lines on stdin.
# Exit 0 only when bridge=up and gateway=up.
set -euo pipefail

bridge=""
gateway=""

while IFS= read -r line || [[ -n "$line" ]]; do
  case "$line" in
    bridge=*) bridge="${line#bridge=}" ;;
    gateway=*) gateway="${line#gateway=}" ;;
  esac
done

if [[ "$bridge" == "up" && "$gateway" == "up" ]]; then
  exit 0
fi
exit 1
