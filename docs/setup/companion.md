# Install the computer companion

The companion runs Codex on your computer and dials out to your relay box. The
box passes the launcher's sealed TLS stream through without opening it. The
companion does not upload your ChatGPT sign-in or copy it to Android.

## Before you start

You need:

- a macOS, Windows, or Linux computer that is normally on;
- a relay box you control, deployed from this repository's
  `Dockerfile.relaybox` and `fly.toml` by following the
  [relay-box deployment guide](relay-box.md);
- the relay box host, pinned public key, and registration secret;
- Codex installed and signed in on that computer;
- one or more folders you are willing to let the phone select for Codex work;
- the companion archive for your operating system and CPU.

Windows and Linux support is experimental until their native lifecycle and
real-Codex smoke tests pass. macOS installation is tested, but the live private
ChatGPT Desktop check still requires an unlocked Mac.

## 1. Verify and extract the download

Download the archive and `SHA256SUMS` from the same release. Verify before you
extract it:

```bash
sha256sum -c SHA256SUMS --ignore-missing
```

On macOS, use `shasum -a 256 <archive>` and compare it with the matching line.
On Windows PowerShell, run
`Get-FileHash .\codex-launcher_0.1.0-alpha.1_windows_amd64.zip -Algorithm SHA256`
and compare the printed hash with the archive's line in `SHA256SUMS`.
The Windows archive contains `codex-launcher.exe`; macOS and Linux archives
contain `codex-launcher`. Each archive also contains checksum and provenance
files for safe local replacement.

Desktop archives are unsigned technical alpha builds. macOS Gatekeeper and
Windows SmartScreen can warn when you open them. Do not disable either system
globally. Continue only after the checksum matches a release you trust.

## 2. Find the setup values

Get the box's public host from Fly:

```bash
fly status --app YOUR_RELAY_APP
```

Use the pinned key printed once by the relay box at its first start. Keep the
registration secret out of source files, shell history, chat, and issue reports.
It must be at least 16 characters and contain no control characters. Store it as
a Fly secret named `RELAYBOX_SECRET`; do not put it in `fly.toml`.

Find the Codex executable:

```bash
command -v codex
```

The PowerShell equivalent is `(Get-Command codex).Source`. Use absolute paths
for Codex and every approved project folder.

## 3. Save setup and install the user service

Run setup from the extracted folder. Repeat all three project flags for each
additional folder:

```bash
./codex-launcher setup \
  --computer-name "My computer" \
  --box-host "YOUR_RELAY_APP.fly.dev" \
  --mac-port 443 \
  --phone-port 8443 \
  --pinned-key "BASE64_PIN_FROM_RELAY_BOX" \
  --relay-secret "SECRET_STORED_IN_FLY" \
  --codex-binary "/absolute/path/to/codex" \
  --project-id "main" \
  --project-name "Main project" \
  --project-path "/absolute/path/to/project"

./codex-launcher install
./codex-launcher status
./codex-launcher doctor
```

On Windows PowerShell, use PowerShell's backtick line continuation and the
current-folder prefix:

```powershell
.\codex-launcher.exe setup `
  --computer-name "My computer" `
  --box-host "YOUR_RELAY_APP.fly.dev" `
  --mac-port 443 `
  --phone-port 8443 `
  --pinned-key "BASE64_PIN_FROM_RELAY_BOX" `
  --relay-secret "SECRET_STORED_IN_FLY" `
  --codex-binary "C:\absolute\path\to\codex.exe" `
  --project-id "main" `
  --project-name "Main project" `
  --project-path "C:\absolute\path\to\project"

.\codex-launcher.exe install
.\codex-launcher.exe status
.\codex-launcher.exe doctor
```

Setup checks that the relay box is reachable, its pinned key matches exactly,
and it accepts the registration secret before saving anything. A mismatch fails
closed; there is no direct-listener fallback. Installation is per-user:
LaunchAgent on macOS, systemd user service on Linux, and a limited current-user
scheduled task on Windows.

Rerun `setup` with the complete folder list whenever you change the approved
folders. A running service is stopped, updated, and restarted. If that restart
fails, the prior config is restored.

## 4. Pair the phone

If this phone was paired to the removed direct/Tailscale address, revoke that
old device first. Re-pairing changes only the phone's saved route and keys; it
does not delete Codex tasks on the computer.

Create a one-time link:

```bash
./codex-launcher pair
```

```powershell
.\codex-launcher.exe pair
```

Enter the printed `codex-launcher://pair?...` link on the phone. The link is a
short-lived secret: do not paste it into chat, logs, or an issue. Only one phone
is paired in V1. Inspect or revoke it with:

```bash
./codex-launcher devices
./codex-launcher revoke DEVICE_ID
```

```powershell
.\codex-launcher.exe devices
.\codex-launcher.exe revoke DEVICE_ID
```

## Maintenance

To install a verified replacement, extract the new archive and keep its
`.sha256` and `.provenance.json` files beside the new executable:

```bash
./codex-launcher install --replace /absolute/path/to/new/codex-launcher
./codex-launcher rollback
```

From the folder containing the installed Windows command, use:

```powershell
.\codex-launcher.exe install --replace C:\absolute\path\to\new\codex-launcher.exe
.\codex-launcher.exe rollback
```

One successful prior version is retained for rollback. Windows can report that
maintenance was scheduled; wait for the installed command to exit, then check
`status` and `doctor`.

To remove the service, installed binary, config, pairing keys, and local
companion state:

```bash
./codex-launcher uninstall
```

```powershell
.\codex-launcher.exe uninstall
```

Uninstalling the computer does not delete Codex tasks stored by Codex itself.

## Troubleshooting

- `Can't reach the relay box` on Android: check the phone's internet connection
  and `fly status --app YOUR_RELAY_APP`. Confirm the dedicated public address and
  raw TCP services still point to Mac port 443 and phone port 8443.
- `Computer offline` on Android: the phone reached the box, but the companion's
  control line is absent. Confirm the computer is awake and
  `./codex-launcher doctor` passes. On Windows PowerShell, run
  `.\codex-launcher.exe doctor`.
- `The relay box is unreachable, or its pinned key does not match`: check
  `--box-host`, `--mac-port`, and `--pinned-key`. Never bypass the pin.
- `The relay box rejected the registration secret`: set the same strong value in
  Fly's `RELAYBOX_SECRET` and `--relay-secret`, then rerun setup.
- `Desktop integration needs an update`: the installed ChatGPT Desktop build is
  outside the compatibility check. Update Codex Launcher or use a supported
  build; the launcher fails closed instead of controlling an unknown interface.
- `Codex is unavailable on this computer`: run `codex --version`, sign in to
  Codex/ChatGPT on the computer, then rerun `doctor`.
