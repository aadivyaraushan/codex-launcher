#!/bin/bash
# Serialize Pixel 9 (adb) access between parallel agents.
# Usage: scripts/pixel-lock.sh <timeout-seconds> <command...>
# Example: scripts/pixel-lock.sh 300 adb shell am start -a android.intent.action.VIEW -d "https://open.spotify.com"
#
# Uses mkdir as the lock because macOS has no flock. A lock older than
# 900 seconds is treated as abandoned and taken over.

LOCK_DIR="/tmp/codex-launcher-pixel.lock"
TIMEOUT="${1:-300}"
shift

waited=0
while ! mkdir "$LOCK_DIR" 2>/dev/null; do
  if [ -d "$LOCK_DIR" ]; then
    age=$(( $(date +%s) - $(stat -f %m "$LOCK_DIR" 2>/dev/null || date +%s) ))
    if [ "$age" -gt 900 ]; then
      echo "pixel-lock: stale lock (${age}s), taking over" >&2
      rm -rf "$LOCK_DIR"
      continue
    fi
  fi
  if [ "$waited" -ge "$TIMEOUT" ]; then
    echo "pixel-lock: timed out after ${TIMEOUT}s waiting for the phone" >&2
    exit 75
  fi
  sleep 3
  waited=$(( waited + 3 ))
done

trap 'rm -rf "$LOCK_DIR"' EXIT INT TERM
"$@"
