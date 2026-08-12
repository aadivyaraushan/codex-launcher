#!/usr/bin/env bash
# Host-side AVD infra proof: reboot → wait → SSH status.
# This is NOT UI proof. WebChat "Ready to chat" still needs screen-driving screenshots.
set -euo pipefail

ADB_SERIAL="${ADB_SERIAL:-}"
SSH_PORT="${SSH_PORT:-18022}"
SSH_USER="${SSH_USER:-}"
SSH_KEY="${SSH_KEY:-}"
TERMUX_FORWARD_PORT="${TERMUX_FORWARD_PORT:-8022}"
BOOT_TIMEOUT_S="${BOOT_TIMEOUT_S:-180}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
EVIDENCE_DIR="${EVIDENCE_DIR:-$REPO_ROOT/saved-results}"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
EVIDENCE_FILE="${EVIDENCE_FILE:-$EVIDENCE_DIR/phase9-avd-reboot-status-${STAMP}.txt}"

usage() {
  cat <<'EOF'
Usage: proof-reboot-status.sh

Required env:
  ADB_SERIAL   adb device/emulator serial
  SSH_USER     Termux sshd user (e.g. u0_a451)
  SSH_KEY      path to Termux ed25519 private key

Optional:
  SSH_PORT=18022  TERMUX_FORWARD_PORT=8022  BOOT_TIMEOUT_S=180

Prereq: Termux sshd listening on device :8022 (Phase 3 bootstrap). Fresh AVD
must complete that bootstrap before this script can run status.
EOF
}

die() { echo "proof-reboot-status: $*" >&2; exit 1; }

[[ -n "$ADB_SERIAL" ]] || { usage; die "ADB_SERIAL required"; }
[[ -n "$SSH_USER" ]] || { usage; die "SSH_USER required"; }
[[ -n "$SSH_KEY" ]] || { usage; die "SSH_KEY required"; }
[[ -f "$SSH_KEY" ]] || die "SSH_KEY not a file: $SSH_KEY"
command -v adb >/dev/null || die "adb not on PATH"
command -v ssh >/dev/null || die "ssh not on PATH"

adb() { command adb -s "$ADB_SERIAL" "$@"; }

echo "proof-reboot-status: rebooting $ADB_SERIAL"
adb reboot

echo "proof-reboot-status: waiting for boot_completed (timeout ${BOOT_TIMEOUT_S}s)"
deadline=$((SECONDS + BOOT_TIMEOUT_S))
while true; do
  if [[ "$(adb shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" == "1" ]]; then
    break
  fi
  if (( SECONDS >= deadline )); then
    die "timed out waiting for boot_completed"
  fi
  sleep 2
done

echo "proof-reboot-status: device booted — unlock the AVD once if credential-encrypted storage is locked"
echo "proof-reboot-status: re-forwarding tcp:${SSH_PORT} -> device:${TERMUX_FORWARD_PORT}"
adb forward --remove "tcp:${SSH_PORT}" >/dev/null 2>&1 || true
adb forward "tcp:${SSH_PORT}" "tcp:${TERMUX_FORWARD_PORT}"

echo "proof-reboot-status: polling operator-phone-boot status over SSH (up to 120s)"
deadline=$((SECONDS + 120))
code=1
out=""
while (( SECONDS < deadline )); do
  set +e
  out="$(ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
    -o ConnectTimeout=5 \
    -p "$SSH_PORT" "${SSH_USER}@127.0.0.1" \
    'operator-phone-boot status' 2>&1)"
  code=$?
  set -e
  if [[ "$code" -eq 0 ]]; then
    break
  fi
  # Between attempts: wait for Termux:Boot / unlock / sshd (host-side proof script).
  sleep 3
done
mkdir -p "$EVIDENCE_DIR"
{
  echo "stamp=${STAMP}"
  echo "adb_serial=${ADB_SERIAL}"
  echo "exit=${code}"
  printf '%s\n' "$out"
} | tee "$EVIDENCE_FILE"
echo "proof-reboot-status: wrote $EVIDENCE_FILE"
echo "proof-reboot-status: exit=$code"
exit "$code"
