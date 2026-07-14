# Mac companion setup

Date: 2026-07-15

## Purpose

Connect the Codex Launcher companion on this Mac to the private Tailscale network before pairing the Android launcher.

## Saved configuration

- Computer name: `MacBook Pro`
- Tailscale address: `100.91.30.118`
- Service port: `9443`
- Codex executable: `/Users/aadivyar/.local/bin/codex`
- Approved working folder: `/Users/aadivyar`
- Project shown in the launcher: `Home folder`

The companion saved this configuration in `/Users/aadivyar/Library/Application Support/codex-launcher/config.json`. It is owner-only (`0700` directory and `0600` file permissions).

## Verified result

The launchd user service installed successfully on 2026-07-15. Its process listens on `100.91.30.118:9443`, and its own log records a ready mobile runtime with four safe task summaries.

The installed replacement was built from commit `676d8839986a9d79d37bba2a77cc45c7ae612caa` and installed through the verified replacement flow. The installed binary and packaged binary have the same SHA-256:

```text
c656507465b3fd3532b5e36417953d91435c32799f5011592ed37344c079fa08
```

After replacement, the service log showed `catalog listed`, `initial task snapshot ready`, `runtime ready`, and `TLS server starting` without requesting Desktop task history during startup. This keeps an old or unavailable Desktop task from stopping the companion service.

Commands used after building the companion:

```sh
/tmp/codex-launcher-current install
/tmp/codex-launcher-current status
lsof -nP -iTCP:9443 -sTCP:LISTEN
/tmp/codex-launcher-current install --replace /tmp/codex-launcher-replacement/codex-launcher
```

Observed status:

```json
{"computer":"MacBook Pro","state":"configured","pairedDevices":0,"projects":1,"service":{"installed":true,"running":true,"detail":"launch agent is running"}}
```

## Remaining external step

The Pixel must join the same Tailscale network before pairing. The current `doctor` command can falsely fail its self-connection check because this Mac does not complete a connection to its own Tailscale address, even though `lsof` confirms the listener. A real Pixel connection is the required end-to-end check.
