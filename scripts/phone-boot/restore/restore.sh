#!/data/data/com.termux/files/usr/bin/bash
# operator-phone-boot — ensure / status / dry-run for OpenClaw + phone-runtime.
# Runs in Termux. Money-safe: never calls openclaw agent or opens a chat WS.
set -euo pipefail

PREFIX="${PREFIX:-/data/data/com.termux/files/usr}"
STATE_ROOT="/data/data/com.termux/no_backup/operator/runtime"
LOCK="$STATE_ROOT/watchdog.lock"
JOURNAL="$STATE_ROOT/watchdog-state.json"
WATCHDOG="$PREFIX/libexec/operator-runtime-watchdog"
BRIDGE_HOSTPORT="127.0.0.1:9443"
GATEWAY_HOSTPORT="127.0.0.1:18789"
SELF_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PARSER="${OPERATOR_PHONE_BOOT_PARSER:-${SELF_DIR}/parse-status.sh}"

usage() {
  cat <<'EOF'
Usage: operator-phone-boot <ensure|status|dry-run>

  ensure   Start the outer watchdog if it is not already holding the lock.
  status   Report bridge + gateway up/down (money-safe probes). Exit 0 iff both up.
  dry-run  Same probes as status; never starts processes.

Probes (no API spend):
  bridge  — TCP 127.0.0.1:9443 + GET https://127.0.0.1:9443/v1/health
  gateway — TCP 127.0.0.1:18789 only (no WebSocket, no openclaw agent)
EOF
}

tcp_up() {
  local hostport="$1"
  if command -v timeout >/dev/null 2>&1; then
    timeout 2 bash -c "echo >/dev/tcp/${hostport%:*}/${hostport#*:}" >/dev/null 2>&1
  else
    bash -c "echo >/dev/tcp/${hostport%:*}/${hostport#*:}" >/dev/null 2>&1
  fi
}

bridge_up() {
  tcp_up "$BRIDGE_HOSTPORT" || return 1
  if command -v curl >/dev/null 2>&1; then
    local code
    code="$(curl -sk --max-time 2 -o /dev/null -w '%{http_code}' "https://${BRIDGE_HOSTPORT}/v1/health" || true)"
    [[ "$code" == "200" ]]
    return
  fi
  # curl missing: TCP alone is enough for infra health (documented fallback).
  return 0
}

gateway_up() {
  tcp_up "$GATEWAY_HOSTPORT"
}

read_journal() {
  if [[ -f "$JOURNAL" ]]; then
    tr '\n' ' ' <"$JOURNAL" | tr -d '\r'
  else
    echo '{"process":"missing"}'
  fi
}

emit_status() {
  local bridge=down gateway=down
  if bridge_up; then bridge=up; fi
  if gateway_up; then gateway=up; fi
  echo "bridge=${bridge}"
  echo "gateway=${gateway}"
  echo "journal=$(read_journal)"
  if [[ "$bridge" == "down" && "$gateway" == "down" ]]; then
    echo "note=if_termux_was_force_stopped_android_blocks_auto_restart_until_user_opens_termux_once"
  fi
}

cmd_status() {
  local out
  out="$(emit_status)"
  printf '%s\n' "$out"
  if [[ -x "$PARSER" ]]; then
    printf '%s\n' "$out" | "$PARSER"
  else
    echo "$out" | grep -q '^bridge=up$' && echo "$out" | grep -q '^gateway=up$'
  fi
}

cmd_dry_run() {
  # Identical probes; never start watchdog/services.
  cmd_status
}

cmd_ensure() {
  mkdir -p "$STATE_ROOT"
  chmod 700 /data/data/com.termux/no_backup/operator 2>/dev/null || true
  chmod 700 "$STATE_ROOT" 2>/dev/null || true
  if [[ ! -x "$WATCHDOG" ]]; then
    echo "operator-phone-boot: missing watchdog at $WATCHDOG — run install-to-termux.sh first" >&2
    exit 2
  fi
  exec 9>"$LOCK"
  if ! flock -n 9; then
    echo '{"process":"watchdog_already_running"}' >"$JOURNAL"
    echo "operator-phone-boot: watchdog already running"
    exit 0
  fi
  # Release probe lock before starting the long-lived watchdog (it takes its own).
  exec 9>&-
  if command -v termux-wake-lock >/dev/null 2>&1; then
    termux-wake-lock || true
  fi
  nohup "$WATCHDOG" >/dev/null 2>&1 &
  echo "operator-phone-boot: started watchdog pid=$!"
}

main() {
  local cmd="${1:-}"
  case "$cmd" in
    ensure) cmd_ensure ;;
    status) cmd_status ;;
    dry-run) cmd_dry_run ;;
    -h|--help|help|"") usage; [[ -n "$cmd" ]] || exit 2; exit 0 ;;
    *) echo "operator-phone-boot: unknown command: $cmd" >&2; usage; exit 2 ;;
  esac
}

main "$@"
