#!/usr/bin/env bash
# Prove adb VIEW URL quoting: unquoted & truncates; helper dry-run keeps PKCE params.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
HELPER="$ROOT/scripts/adb-open-url.sh"
URL='https://example.com/oauth/authorize?client_id=x&scope=y&code_challenge=z&state=s'

# Mechanism: device shell treats bare & as background (same as adb shell am start -d URL).
bad=$(sh -c "printf '%s\n' $URL" || true)
if [[ "$bad" == *code_challenge* ]]; then
  echo "FAIL: expected unquoted URL to truncate before code_challenge; got: $bad" >&2
  exit 1
fi
if [[ "$bad" != 'https://example.com/oauth/authorize?client_id=x' ]]; then
  echo "FAIL: unexpected truncated form: $bad" >&2
  exit 1
fi

if [[ ! -x "$HELPER" ]]; then
  echo "FAIL: missing executable helper $HELPER" >&2
  exit 1
fi

dry=$("$HELPER" --dry-run 4B230DLAQ001Z5 "$URL")
if [[ "$dry" != *"am start -a android.intent.action.VIEW -d '$URL'"* ]]; then
  echo "FAIL: dry-run missing single-quoted -d URL; got: $dry" >&2
  exit 1
fi
if [[ "$dry" != *code_challenge=z* ]]; then
  echo "FAIL: dry-run dropped code_challenge" >&2
  exit 1
fi

# Quoted form must round-trip through a device-like shell.
good=$(sh -c "printf '%s\n' '$URL'")
if [[ "$good" != "$URL" ]]; then
  echo "FAIL: quoted URL did not round-trip; got: $good" >&2
  exit 1
fi

echo "PASS adb-open-url quoting (truncate + dry-run + round-trip)"
