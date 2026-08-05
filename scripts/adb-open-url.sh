#!/usr/bin/env bash
# Open a VIEW URL on-device without device-shell truncating at &.
# Usage: scripts/adb-open-url.sh [--dry-run] [serial] <url-or-@file>
# Prefer this over: adb shell am start -d "$URL"  (drops PKCE / query params).
set -euo pipefail

DRY=0
if [[ "${1:-}" == "--dry-run" ]]; then
  DRY=1
  shift
fi

if [[ $# -lt 1 ]]; then
  echo "usage: $0 [--dry-run] [serial] <url-or-@file>" >&2
  exit 2
fi

if [[ $# -ge 2 ]]; then
  SERIAL="$1"
  TARGET="$2"
else
  SERIAL="${ANDROID_SERIAL:-4B230DLAQ001Z5}"
  TARGET="$1"
fi

if [[ "$TARGET" == @* ]]; then
  URL=$(tr -d '\n' < "${TARGET#@}")
else
  URL="$TARGET"
fi

if [[ "$URL" == *"'"* ]]; then
  echo "adb-open-url: URL contains a single quote; refuse unsafe wrap" >&2
  exit 2
fi

CMD="am start -a android.intent.action.VIEW -d '$URL'"
if [[ "$DRY" -eq 1 ]]; then
  printf 'adb -s %s shell %s\n' "$SERIAL" "$CMD"
  exit 0
fi

adb -s "$SERIAL" shell "$CMD"
