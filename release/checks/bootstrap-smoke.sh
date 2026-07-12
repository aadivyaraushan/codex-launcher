#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)

run() {
  printf 'bootstrap smoke: %s\n' "$*"
  "$@"
}

cd "$repo_root"
run go test ./...
run "$repo_root/android/gradlew" -p "$repo_root/android" :app:testDebugUnitTest
run "$repo_root/android/gradlew" -p "$repo_root/android" :app:connectedDebugAndroidTest

printf 'bootstrap smoke: all targets passed\n'
