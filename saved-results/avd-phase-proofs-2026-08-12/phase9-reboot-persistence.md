# AVD Phase 9 — reboot persistence proof (2026-08-12/13)

**Device:** AVD `codex_launcher_pixel_9_api_36` (`emulator-5554`), Pixel 9 / API 36  
**Branch tip at merge:** `worktree-phase2-tool-bridge` @ `554126f` (+ local proof artifacts)  
**Account for model spend:** OpenAI key loaded into on-device OpenClaw auth (ssdear-approved spend)

## Result

| Check | Result |
|---|---|
| Pre-reboot bridge `:9443` | UP |
| Pre-reboot gateway `:18789` | UP |
| Reboot via `adb reboot` | OK |
| After open Termux + `operator-phone-boot ensure` | bridge HTTP 200, gateway ready, operator-tools loaded |
| Post-reboot agent smoke (`Reply with exactly: pong`) | see `post-reboot-agent-pong.txt` |

## Honesty / caveats

1. **Auto-start without user open:** After reboot, Termux was opened explicitly, then `operator-phone-boot ensure` started the watchdog. Job **7301** registration previously hung on Termux:API JobScheduler; boot may not recover until Termux is opened once (Android force-stop / credential-encrypted storage rules). Do **not** claim unattended cold-boot without Termux open until Termux:Boot + job 7301 are verified.
2. **Termux `curl` is broken** (missing `SSL_set_quic_tls_transport_params` in libngtcp2). `operator-phone-boot status` can mis-report bridge down because it prefers curl when present. Probes used bash `/dev/tcp` + Debian `curl` inside proot.
3. Emulator default route (`default via 10.0.2.2`) sometimes missing after network flaps; restored with `su 0 ip route add default via 10.0.2.2` when needed for registry/API.

## Stack on AVD (Debian under proot)

- OpenClaw `2026.7.1-2` global install (repaired after incomplete first install + DNS/route outage)
- `operator-phone-runtime` on `:9443` with token/cert under `/var/lib/operator-phone/`
- Plugin `operator-tools` enabled under `~/.openclaw/extensions/operator-tools`
- Runit services: `/etc/operator/services/{openclaw-gateway,phone-runtime}`
- Watchdog: `$PREFIX/libexec/operator-runtime-watchdog` via `operator-phone-boot ensure`

## Artifacts in this directory

- `pre-reboot-status.txt`, `pre-reboot.png`
- `post-reboot-home.png`
- `post-reboot-before-ensure.txt`, `post-reboot-after-ensure.txt`, `post-reboot-after-ensure.png`
- `post-reboot-final-status.txt`, `post-reboot-services-up.png`
- `post-reboot-operator-ui.png`, `post-reboot-webchat.png`
- `post-reboot-agent-pong.txt`

## Next

- Register job 7301 successfully; verify Termux:Boot path on this AVD image
- Fix Termux curl SSL (pkg reinstall openssl/ngtcp2) so `operator-phone-boot status` is trustworthy
- Screen-driven WebChat “Ready to chat” PASS with timestamped taps (if not already clear from webchat screenshot)


## Operator UI after reboot

Screenshot `post-reboot-operator-ui.png` shows the chat-first Operator home with:
- "Link local runtime" / "What do you want done?"
- Error: `http_403:attestation: certificate chain invalid: x509: certificate signed by unknown authority`

That means the Android app reached something on loopback but does not trust the phone-runtime TLS cert yet (attestation/pinning). Infra ports (`:9443` / `:18789`) are still up under runit.
