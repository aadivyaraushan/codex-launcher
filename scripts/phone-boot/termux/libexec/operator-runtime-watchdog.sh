#!/data/data/com.termux/files/usr/bin/bash
# Outer Termux watchdog: exclusive flock, state journal, proot runsvdir with capped backoff.
set -euo pipefail
STATE_ROOT="/data/data/com.termux/no_backup/operator/runtime"
LOCK="$STATE_ROOT/watchdog.lock"
JOURNAL="$STATE_ROOT/watchdog-state.json"
mkdir -p "$STATE_ROOT"
chmod 700 /data/data/com.termux/no_backup/operator 2>/dev/null || true
chmod 700 "$STATE_ROOT"
exec 9>"$LOCK"
if ! flock -n 9; then
  echo '{"process":"watchdog_already_running"}' >"$JOURNAL"
  exit 0
fi
attempt=0
backoff=1
while true; do
  attempt=$((attempt + 1))
  printf '{"process":"starting","attempt":%s,"backoff_s":%s,"ts":%s}\n' "$attempt" "$backoff" "$(date +%s)" >"$JOURNAL"
  set +e
  proot-distro login debian -- /usr/bin/runsvdir /etc/operator/services
  code=$?
  set -e
  printf '{"process":"exited","attempt":%s,"exit_code":%s,"ts":%s}\n' "$attempt" "$code" "$(date +%s)" >"$JOURNAL"
  sleep "$backoff"
  if [ "$backoff" -lt 60 ]; then
    next=$((backoff * 2))
    if [ "$next" -gt 60 ]; then next=60; fi
    if [ "$backoff" -eq 1 ]; then next=2; fi
    # capped sequence 1/2/4/8/16/30/60
    case "$backoff" in
      1) backoff=2 ;;
      2) backoff=4 ;;
      4) backoff=8 ;;
      8) backoff=16 ;;
      16) backoff=30 ;;
      *) backoff=60 ;;
    esac
  fi
done
