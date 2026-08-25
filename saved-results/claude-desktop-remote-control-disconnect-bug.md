# Claude desktop: `/remote-control` disconnect always errors

**Date:** 2026-07-31
**What this is:** diagnosis + a ready-to-file bug report for a Claude desktop app bug (unrelated to this repo — filed here because it's the current project's `saved-results/`).
**Status:** root cause confirmed by reading the shipped code. Local workaround patch built and verified; see "Local patch" below.

**Not filed — already reported upstream.** At least 10 open issues cover this, most precisely
[anthropics/claude-code#77915](https://github.com/anthropics/claude-code/issues/77915)
("toggle-off path missing null guard", 15 comments). That thread already contains every
finding below, including the `c?.session_url` guard, the missing `remoteControlEnabled` reset
in the `catch`, and the CLI's empty-payload reply on disable. Also open: #82702, #82567,
#82583, #82262, #81228, #81155, #78336, #78938. Keep this page for the local patch record.

---

## Environment

| Thing | Value |
|---|---|
| Claude desktop app | 1.24012.9 (`/Applications/Claude.app`) |
| Claude Code CLI | 2.1.220 (`~/.local/share/claude/versions/2.1.220`) |
| OS | macOS 15 (Darwin 24.5.0), Apple Silicon |

## Symptom

Turning **off** Remote Control from the desktop app prints:

```
Remote Control failed to disconnect: Cannot read properties of undefined (reading 'session_url')
```

It happens every time, on every session. Turning Remote Control *on* works fine.

## Root cause

The two halves of the feature disagree about whether the "disable" reply carries a payload.

**CLI side** (`~/.local/share/claude/versions/2.1.220`, `remote_control` control-request handler). The enable branch responds with a payload; the disable branch responds with nothing:

```js
// enabled === true
Hn(dt, { session_url: ..., connect_url: ..., environment_id: ... });

// enabled === false
if (z) { /* ...teardown... */ }
Hn(dt);            // <-- no payload
```

**SDK shim** (both binaries) returns the payload field directly:

```js
async enableRemoteControl(e, t) {
  return (await this.request({ subtype: "remote_control", enabled: e, ... })).response;
}
```

With no payload, `.response` is `undefined`.

**Desktop app** (`/Applications/Claude.app/Contents/Resources/app.asar`, `handleRemoteControlCommand`) then dereferences that value without a guard:

```js
const c = await e.query.enableRemoteControl(o, o ? e.title : void 0),
      l = (a = c.session_url) == null ? void 0 : a.split("/").filter(Boolean).pop();
//         ^^^ `c` is undefined on the disable path -> TypeError
```

Note the inner access is already optional-chained (`c.session_url?.split(...)`), but `c` itself is not.

## Consequences

1. **The disconnect itself actually succeeds.** `await z.teardown({reason:"remote_control_disabled"})` runs *before* the empty response is sent, so the bridge is genuinely torn down. The error is entirely in the app's post-processing.
2. **The app's own state is left wrong.** In the `catch`, only the *connect* path resets state. The disconnect path just prints the message:

   ```js
   o ? (e.remoteControlEnabled = !1, e.remoteControlAutoEnabled = void 0,
        e.bridgeSessionId = void 0, e.bridgeSessionUrl = void 0, ...)
     : i || this.emitSyntheticAssistantMessage(e, `Remote Control failed to disconnect: ${l}`, ...)
   ```

   So `remoteControlEnabled` stays `true` and `bridgeSessionUrl` stays set. The UI keeps showing the session as remote-controlled after it has been disconnected, and it stays that way until the session is restarted.
3. `handleRemoteControlCommand` returns `{ok: false}`, so `toggleRemoteControl` reports failure to any caller for what was a successful teardown.

## Suggested fix

One character in the desktop app:

```js
l = (a = c?.session_url) == null ? void 0 : a.split("/").filter(Boolean).pop();
```

Worth doing as well, independently:

- Have the CLI's disable branch respond with `{}` instead of no payload, so the SDK's `.response` is an object on both paths.
- Reset `remoteControlEnabled` / `bridgeSessionUrl` in the `catch` on the disconnect path too, so a genuine disconnect failure doesn't strand the UI in "connected".

## Steps to reproduce

1. Open a session in the Claude desktop app and send a message (Remote Control needs an active query).
2. Run `/remote-control` to enable it. It connects; a claude.ai/code URL is produced.
3. Run `/remote-control` again to disable it.
4. The error appears. The bridge is down, but the session still displays as remote-controlled.

## How this was diagnosed

No debugger — the shipped bundles were read directly:

```bash
strings -a /Applications/Claude.app/Contents/Resources/app.asar | grep -o "failed to disconnect.\{0,400\}"
strings -a ~/.local/share/claude/versions/2.1.220 | grep -o 'dt.request.subtype==="remote_control".\{0,2600\}'
```

---

## Local patch (applied on this machine)

`app.asar` is `root:wheel 644`, so installing needs `sudo`. The patched archive was built in the session scratchpad and verified before install.

The edit is **length-preserving** so the asar header (which stores each file's size and offset) stays valid without repacking:

- `(a=c.session_url)` → `(a=c?.session_url)` — **+1 byte**
- `` `Remote control enabled: ${c.session_url}` `` → `` `Remote control enabled:${c.session_url}` `` — **−1 byte** (one space dropped from a log line in the same bundled file, 749 bytes away)

Verified: total size unchanged (38,986,429 bytes both ways), and `cmp -l` shows the only differing bytes are in the 769-byte window between the two edits — everything before and after is byte-identical.

Script: `patch_asar.py` (kept in the session scratchpad; reproduced by the steps above).

Install:

```bash
sudo cp app.asar.patched /Applications/Claude.app/Contents/Resources/app.asar
```

Backup: `~/Desktop/app.asar.backup-remotecontrol`

Restore:

```bash
sudo cp ~/Desktop/app.asar.backup-remotecontrol /Applications/Claude.app/Contents/Resources/app.asar
```

### Caveats

- Modifying anything under `Contents/` breaks the app's code signature seal. An already-installed, already-launched app normally keeps running, but this is the main risk — the backup above is the undo.
- **Every app update overwrites this.** The patch has to be reapplied after each one, or just dropped once the fix ships upstream.
- Quit Claude before copying, then relaunch.
