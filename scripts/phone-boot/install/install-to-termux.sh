#!/data/data/com.termux/files/usr/bin/bash
# Install Phase 9 boot persistence into Termux + proot Debian.
# Idempotent. Paths only — never writes secret values.
set -euo pipefail

PREFIX="${PREFIX:-/data/data/com.termux/files/usr}"
HOME_DIR="${HOME:-/data/data/com.termux/files/home}"
PKG_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

BOOT_SRC="$PKG_ROOT/termux/boot/10-operator-runtime.sh"
WATCH_SRC="$PKG_ROOT/termux/libexec/operator-runtime-watchdog.sh"
RESTORE_SRC="$PKG_ROOT/restore/restore.sh"
PARSE_SRC="$PKG_ROOT/restore/parse-status.sh"
SERVICES_SRC="$PKG_ROOT/services"

BOOT_NAME="10-operator-runtime"
WATCH_DST="$PREFIX/libexec/operator-runtime-watchdog"
RESTORE_LIB="$PREFIX/libexec/operator-phone-boot-restore"
PARSE_DST="$PREFIX/libexec/operator-phone-boot-parse-status"
RESTORE_DST="$PREFIX/bin/operator-phone-boot"
JOB_ID=7301

log() { printf '[phone-boot-install] %s\n' "$*"; }
die() { printf '[phone-boot-install] ERROR: %s\n' "$*" >&2; exit 1; }

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"
}

install_boot_scripts() {
  local dir
  for dir in "$HOME_DIR/.termux/boot" "$HOME_DIR/.config/termux/boot"; do
    mkdir -p "$dir"
    install -m 0755 "$BOOT_SRC" "$dir/$BOOT_NAME"
    log "installed boot script -> $dir/$BOOT_NAME"
  done
}

install_watchdog_and_cli() {
  mkdir -p "$PREFIX/libexec" "$PREFIX/bin"
  install -m 0755 "$WATCH_SRC" "$WATCH_DST"
  install -m 0755 "$RESTORE_SRC" "$RESTORE_LIB"
  install -m 0755 "$PARSE_SRC" "$PARSE_DST"
  cat >"$RESTORE_DST" <<EOF
#!/data/data/com.termux/files/usr/bin/bash
export OPERATOR_PHONE_BOOT_PARSER="$PARSE_DST"
exec bash "$RESTORE_LIB" "\$@"
EOF
  chmod 0755 "$RESTORE_DST"
  log "installed watchdog -> $WATCH_DST"
  log "installed CLI -> $RESTORE_DST"
}

install_debian_services() {
  require_cmd proot-distro
  proot-distro login debian -- /bin/bash -c \
    'mkdir -p /etc/operator/services /var/log/operator /var/lib/operator-phone'
  for svc in openclaw-gateway phone-runtime; do
    tar -C "$SERVICES_SRC" -cf - "$svc" | proot-distro login debian -- /bin/bash -c \
      "mkdir -p /etc/operator/services && tar -C /etc/operator/services -xf - && chmod 755 /etc/operator/services/$svc/run /etc/operator/services/$svc/log/run"
    log "installed debian service -> /etc/operator/services/$svc"
  done
  # Prefer canonical phone-runtime; retire legacy operator-phone-runtime dir if present.
  proot-distro login debian -- /bin/bash -c '
    set -euo pipefail
    if [ -d /etc/operator/services/operator-phone-runtime ]; then
      if [ -e /etc/operator/services/phone-runtime ]; then
        rm -rf /etc/operator/services/operator-phone-runtime
      else
        mv /etc/operator/services/operator-phone-runtime /etc/operator/services/phone-runtime
      fi
    fi
  ' || true
}

register_job() {
  require_cmd termux-job-scheduler
  termux-job-scheduler \
    --script "$WATCH_DST" \
    --job-id "$JOB_ID" \
    --period-ms 900000 \
    --network none \
    --battery-not-low false \
    --persisted true
  local listing
  listing="$(termux-job-scheduler --list 2>/dev/null || true)"
  if ! printf '%s\n' "$listing" | grep -q "$JOB_ID"; then
    die "job $JOB_ID not listed after registration; output was: ${listing:-<empty>}"
  fi
  log "registered persisted job id=$JOB_ID (verify: termux-job-scheduler --list | grep $JOB_ID)"
}

main() {
  [[ "$(uname -o 2>/dev/null || true)" == "Android" ]] || log "warning: not running on Android; continuing for packaging checks"
  require_cmd install
  [[ -f "$BOOT_SRC" ]] || die "missing $BOOT_SRC"
  [[ -f "$WATCH_SRC" ]] || die "missing $WATCH_SRC"
  [[ -f "$RESTORE_SRC" ]] || die "missing $RESTORE_SRC"
  require_cmd termux-job-scheduler
  command -v curl >/dev/null 2>&1 || log "warning: curl not installed; status falls back to TCP-only for bridge (pkg install curl)"
  install_boot_scripts
  install_watchdog_and_cli
  install_debian_services
  register_job
  log "done. Next: operator-phone-boot ensure && operator-phone-boot status"
  log "Force-stop note: Android blocks auto-restart until the user opens Termux once."
}

main "$@"
