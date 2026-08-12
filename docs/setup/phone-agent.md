# OpenClaw phone agent setup

Operator’s primary path is an **OpenClaw agent that runs on the phone**
(Termux + proot Debian). This repo supplies the Go tool bridge, the OpenClaw
plugin, boot/restore scripts, and the Android chat / approval UI.

For the computer-Codex lane (companion + relay), see
[companion.md](companion.md) and [relay-box.md](relay-box.md).

## Pieces

| Piece | In this repo | On the phone (typical) |
|---|---|---|
| OpenClaw gateway + agent | installed in Debian (Phase 1) | supervised under `/etc/operator/services/openclaw-gateway` |
| Phone runtime / tool bridge | `companion/cmd/operator-phone-runtime` (+ `phoneruntime/agentbridge`) | loopback HTTPS (e.g. `:9443`), token + cert files |
| OpenClaw plugin | [`agentbridge/openclaw-plugin/`](../../agentbridge/openclaw-plugin/) | installed into the OpenClaw extensions tree |
| Boot / restore | [`scripts/phone-boot/`](../../scripts/phone-boot/) | Termux boot scripts, watchdog, job **7301** |
| Chat / approvals UI | `android/` | launcher home + task threads |

## Config rules (secrets)

OpenClaw plugin config must store **paths only**, for example:

- `bridgeUrl`: `https://127.0.0.1:9443` (loopback)
- `tokenPath`: path to the bridge bearer token file (mode `0600`)
- `certPath`: path to the runtime TLS certificate PEM

Never commit token values, private keys, or keystores. Never log message text
or people’s names — ids only. See the plugin schema in
`agentbridge/openclaw-plugin/openclaw.plugin.json`.

## Install boot persistence

After OpenClaw + `operator-phone-runtime` are present in Debian:

```sh
pkg install termux-api curl
bash scripts/phone-boot/install/install-to-termux.sh
operator-phone-boot ensure
operator-phone-boot status
```

Full Termux:Boot notes, force-stop honesty, and AVD infra runbook:
[scripts/phone-boot/README.md](../../scripts/phone-boot/README.md).

## Plugin development

```sh
cd agentbridge/openclaw-plugin
npm test
```

`npm run sync-tools` regenerates tool manifests from a live bridge when you
intentionally refresh the contract. Do not paste live tokens into the repo.

## Device proofs

Prefer a **Pixel-like AVD** (API 36 / Android 16 when available). Real-user UI
proof is adb screen-driving only (taps, swipes, typed text, screenshots) —
not intents, test hooks, or curl standing in for the chat UI.
