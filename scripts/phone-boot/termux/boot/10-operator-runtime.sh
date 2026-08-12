#!/data/data/com.termux/files/usr/bin/bash
# Termux boot script: ~/.config/termux/boot/10-operator-runtime
# Acquires the Termux wake lock and execs the outer watchdog.
set -euo pipefail
PREFIX="${PREFIX:-/data/data/com.termux/files/usr}"
if command -v termux-wake-lock >/dev/null 2>&1; then
  termux-wake-lock || true
fi
exec "$PREFIX/libexec/operator-runtime-watchdog"
