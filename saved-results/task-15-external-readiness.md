# Task 15 external readiness

Date: 2026-07-14

Purpose: record the current real-environment gates after the clean Android 16
emulator audit. This is a readiness check only; it does not claim any live
Tailscale, physical-device, or model-backed result.

## Checked state

- Worktree: `/Users/aadivyar/Documents/Codex/2026-07-12/uf-u-implementation`
- Branch head: `f79b8d1`
- `adb devices -l`: only `emulator-5554` was connected. No physical Pixel was
  available through ADB.
- `command -v tailscale`: no executable was found. The official Tailscale
  client is not installed on this Mac.
- Attempted `brew install --cask tailscale`: Homebrew downloaded official
  Tailscale 1.98.8, then stopped before installation because macOS required an
  interactive administrator password for `sudo installer`. Homebrew removed
  the staged cask files. After a successful manual install, macOS may also ask
  the user to approve Tailscale's system extension in Privacy & Security.
- Codex CLI: `/Users/aadivyar/.local/bin/codex`, version `0.144.1`.
- ChatGPT Desktop process is running. Its installed build from
  `/Applications/ChatGPT.app/Contents/Info.plist` is `26.707.72221`.
- The desktop follower adapter deliberately pins build `26.707.51957` in
  `companion/internal/codex/desktopipc/client.go`. It will therefore fail
  closed until the newer private bridge is read-verified.
- A read-only inspection of the installed `app.asar` found the follower method
  names the companion relies on, including `thread-follower-load-complete-history`,
  `thread-follower-start-turn`, `thread-follower-steer-turn`, and the matching
  request and response names. This shows that the newer bundle still contains
  that interface, but it does **not** prove its message versions, parameters,
  or responses. The compatibility pin remains unchanged on purpose.
- QEMU system binaries are installed for both AArch64 and x86_64. No guest
  image or licensed Windows media was selected or started.
- Local cross-compiles completed for Linux and Windows on amd64 and arm64.
  Generated binaries were written under `/tmp` or removed immediately; the
  worktree was clean before this note.

## Safe next checks

1. Install official Tailscale on the Mac with an administrator password, approve
   its system extension if macOS asks, then sign in on the Mac and Pixel using
   the user's own tailnet.
2. Connect the physical Pixel 9 by USB and approve USB debugging.
3. With the Mac unlocked, provide an existing harmless Codex thread ID so the
   read-only Desktop follower probe can compare the installed build's bridge
   behavior before any compatibility pin changes.
4. Before any model-backed write, identify the ChatGPT account/workspace and
   explicitly approve the expected small number of calls or quota use.
5. Before Windows VM testing, select licensed media and approve any account or
   cost surface.

## Reproduce

```sh
adb devices -l
command -v tailscale
codex --version
/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' /Applications/ChatGPT.app/Contents/Info.plist
strings /Applications/ChatGPT.app/Contents/Resources/app.asar | rg -o 'thread-follower-[a-z0-9-]+' | sort -u
GOOS=linux GOARCH=amd64 go build ./companion/cmd/codex-launcher
GOOS=windows GOARCH=amd64 go build ./companion/cmd/codex-launcher
GOOS=linux GOARCH=arm64 go build -o /tmp/codex-launcher-linux-arm64 ./companion/cmd/codex-launcher
GOOS=windows GOARCH=arm64 go build -o /tmp/codex-launcher-windows-arm64.exe ./companion/cmd/codex-launcher
```
