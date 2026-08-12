#!/usr/bin/env bash
# Phase 9 phone-boot contract tests. Money-safe: never calls openclaw agent.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
FAIL=0

fail() {
  echo "FAIL: $*" >&2
  FAIL=$((FAIL + 1))
}

pass() {
  echo "PASS: $*"
}

require_file() {
  local path="$1"
  if [[ ! -f "$path" ]]; then
    fail "missing file $path"
    return 1
  fi
  pass "exists $(basename "$(dirname "$path")")/$(basename "$path")"
}

require_exec() {
  local path="$1"
  require_file "$path" || return 1
  if [[ ! -x "$path" ]]; then
    fail "not executable: $path"
    return 1
  fi
}

require_contains() {
  local path="$1" needle="$2"
  if ! grep -qF -- "$needle" "$path"; then
    fail "$path missing required fragment: $needle"
    return 1
  fi
}

forbid_regex() {
  local path="$1" re="$2"
  if grep -nE -- "$re" "$path" >/dev/null 2>&1; then
    fail "$path matched forbidden secret-like pattern: $re"
    return 1
  fi
}

echo "== service templates =="
for svc in openclaw-gateway phone-runtime; do
  require_exec "$ROOT/services/$svc/run" || true
  require_exec "$ROOT/services/$svc/log/run" || true
done
require_contains "$ROOT/services/openclaw-gateway/run" "exec openclaw gateway" || true
require_contains "$ROOT/services/openclaw-gateway/log/run" "svlogd" || true
require_contains "$ROOT/services/openclaw-gateway/log/run" "/var/log/operator/openclaw-gateway" || true
require_contains "$ROOT/services/phone-runtime/run" "exec /usr/local/bin/operator-phone-runtime" || true
require_contains "$ROOT/services/phone-runtime/run" "-root /var/lib/operator-phone" || true
require_contains "$ROOT/services/phone-runtime/run" "127.0.0.1:9443" || true
require_contains "$ROOT/services/phone-runtime/log/run" "/var/log/operator/phone-runtime" || true

echo "== secrets contract (paths only) =="
while IFS= read -r -d '' f; do
  forbid_regex "$f" 'agentbridge-token[[:space:]]*=[[:space:]]*['\''"]?[0-9a-fA-F]{32,}' || true
  forbid_regex "$f" 'Bearer[[:space:]]+[A-Za-z0-9._-]{16,}' || true
  forbid_regex "$f" 'sk-[A-Za-z0-9]{10,}' || true
done < <(find "$ROOT" -type f \( -name '*.sh' -o -name 'run' -o -name 'README.md' \) -print0)

echo "== termux boot + watchdog =="
require_exec "$ROOT/termux/boot/10-operator-runtime.sh" || true
require_exec "$ROOT/termux/libexec/operator-runtime-watchdog.sh" || true
require_contains "$ROOT/termux/boot/10-operator-runtime.sh" "termux-wake-lock" || true
require_contains "$ROOT/termux/boot/10-operator-runtime.sh" "operator-runtime-watchdog" || true
require_contains "$ROOT/termux/libexec/operator-runtime-watchdog.sh" "flock -n" || true
require_contains "$ROOT/termux/libexec/operator-runtime-watchdog.sh" "runsvdir /etc/operator/services" || true
require_contains "$ROOT/termux/libexec/operator-runtime-watchdog.sh" "proot-distro login debian" || true

echo "== restore CLI =="
require_exec "$ROOT/restore/restore.sh" || true
require_exec "$ROOT/install/install-to-termux.sh" || true
require_contains "$ROOT/restore/restore.sh" "dry-run" || true
require_contains "$ROOT/restore/restore.sh" "127.0.0.1:9443" || true
require_contains "$ROOT/restore/restore.sh" "127.0.0.1:18789" || true
require_contains "$ROOT/restore/restore.sh" "/v1/health" || true
# Must not spend money: flag real invocations, not prose that says "never run openclaw agent".
if grep -nE '^[[:space:]]*openclaw[[:space:]]+agent|[[:space:]]chat\.send' "$ROOT/restore/restore.sh" >/dev/null 2>&1; then
  fail "restore.sh must not invoke openclaw agent or chat.send"
else
  pass "restore.sh has no paid agent/chat calls"
fi

echo "== bash -n =="
while IFS= read -r -d '' f; do
  if ! bash -n "$f"; then
    fail "bash -n $f"
  else
    pass "bash -n $(basename "$f")"
  fi
done < <(find "$ROOT" -type f -name '*.sh' -print0)

echo "== status parser fixtures =="
PARSER="$ROOT/restore/parse-status.sh"
require_exec "$PARSER" || true
if [[ -x "$PARSER" ]]; then
  if "$PARSER" <"$ROOT/test/fixtures/status-up.txt"; then
    pass "status-up exits 0"
  else
    fail "status-up should exit 0"
  fi
  if "$PARSER" <"$ROOT/test/fixtures/status-down.txt"; then
    fail "status-down should exit non-zero"
  else
    pass "status-down exits non-zero"
  fi
fi

echo "== docs =="
require_file "$ROOT/README.md" || true
require_contains "$ROOT/README.md" "Termux:Boot" || true
require_contains "$ROOT/README.md" "operator-phone-boot" || true
require_contains "$ROOT/README.md" "7301" || true
require_file "$ROOT/avd/proof-reboot-status.sh" || true

if [[ "$FAIL" -ne 0 ]]; then
  echo "RESULT: $FAIL failure(s)" >&2
  exit 1
fi
echo "RESULT: OK"
exit 0
