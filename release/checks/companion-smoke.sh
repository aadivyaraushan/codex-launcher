#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 9 ]]; then
  printf 'usage: %s COMPANION REPLACEMENT CODEX_BINARY BOX_HOST MAC_PORT PHONE_PORT PINNED_KEY RELAY_SECRET PROJECT_DIR\n' "$0" >&2
  exit 2
fi

companion=$1
replacement=$2
codex_binary=$3
box_host=$4
mac_port=$5
phone_port=$6
pinned_key=$7
relay_secret=$8
project_dir=$9

for executable in "$companion" "$replacement" "$codex_binary"; do
  if [[ ! -f "$executable" ]]; then
    printf 'companion smoke: missing local file: %s\n' "$executable" >&2
    exit 2
  fi
done
if [[ ! -d "$project_dir" ]]; then
  printf 'companion smoke: project directory is missing\n' >&2
  exit 2
fi

smoke_home=$(mktemp -d)
installed=0
cleanup() {
	if [[ $installed -eq 1 ]]; then
		"$installed_companion" uninstall >/dev/null 2>&1 || true
  fi
  rm -rf "$smoke_home"
}
trap cleanup EXIT

export HOME="$smoke_home"
export XDG_CONFIG_HOME="$smoke_home/.config"
case "$(uname -s)" in
	Darwin) installed_companion="$HOME/Library/Application Support/codex-launcher/bin/codex-launcher" ;;
	Linux) installed_companion="$XDG_CONFIG_HOME/codex-launcher/bin/codex-launcher" ;;
	*) printf 'companion smoke: unsupported POSIX platform\n' >&2; exit 2 ;;
esac

"$companion" version
"$companion" setup \
  --computer-name "Codex Launcher smoke" \
  --box-host "$box_host" \
  --mac-port "$mac_port" \
  --phone-port "$phone_port" \
  --pinned-key "$pinned_key" \
  --relay-secret "$relay_secret" \
  --codex-binary "$codex_binary" \
  --project-id smoke \
  --project-name "Smoke project" \
  --project-path "$project_dir"
"$companion" install
installed=1
"$installed_companion" status
"$installed_companion" doctor
"$installed_companion" install --replace "$replacement"
"$installed_companion" status
"$installed_companion" doctor
"$installed_companion" rollback
"$installed_companion" status
"$installed_companion" doctor
"$installed_companion" uninstall
installed=0

printf 'companion smoke: install, replace, rollback, and uninstall passed\n'
