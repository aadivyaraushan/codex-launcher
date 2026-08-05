#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT="$ROOT/saved-results/wave4-provenance-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "$OUT"
{
  echo "commit=$(git -C "$ROOT" rev-parse HEAD)"
  echo "dirty=$(git -C "$ROOT" status --porcelain | wc -l | tr -d ' ')"
  if [[ -f "$ROOT/artifacts/phone-runtime/operator-phone-runtime-linux-arm64" ]]; then
    shasum -a 256 "$ROOT/artifacts/phone-runtime/operator-phone-runtime-linux-arm64"
  else
    echo "phone-runtime:MISSING"
  fi
  if [[ -f "$ROOT/.well-known/assetlinks.json" ]]; then
    shasum -a 256 "$ROOT/.well-known/assetlinks.json"
  fi
} | tee "$OUT/baseline.txt"
echo "$OUT"
