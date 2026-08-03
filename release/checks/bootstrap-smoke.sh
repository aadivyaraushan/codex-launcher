#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)

run() {
  printf 'bootstrap smoke: %s\n' "$*"
  "$@"
}

cd "$repo_root"
run python3 release/checks/protocol/schema_test.py
run go test ./...
run "$repo_root/android/gradlew" -p "$repo_root/android" :app:testDebugUnitTest
run node release/checks/android-device-ready.mjs
run "$repo_root/android/gradlew" -p "$repo_root/android" :app:connectedDebugAndroidTest

printf 'bootstrap smoke: all targets passed\n'
