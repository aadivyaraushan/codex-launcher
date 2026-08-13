# AVD phase proofs — 2026-08-12

Device: AVD `codex_launcher_pixel_9_api_36` (Pixel 9 profile, Android 16 / API 36, google_apis arm64)
Serial: `emulator-5554`
Host: owner Mac; physical Pixel attached earlier was **not** used.

## Done on this AVD
- Booted Pixel-like AVD (screen-driven screenshots below)
- Built + installed `app.codexlauncher` debug APK from `worktree-phase2-tool-bridge` tip `554126f`
- Operator chat-first home visible: title Operator, prompt "What do you want done?", All apps / Android Settings
- Typed `AVD_PROOF_CHAT_UI` into the prompt via `adb shell input` (screen-level; no intents for the UI content itself — launch used `am start` only to open the app under test)
- Sideloaded Termux `v0.118.3` github-debug arm64 APK

## Still open (honest)
- Full OpenClaw + proot Debian + tool-bridge stack is **not** installed on this fresh AVD yet
- Phase 9 reboot → WebChat "Ready to chat" bar still open (needs that stack + Termux:Boot)
- Phases 3–7 on-device agent/tool proofs still open for the same reason
- No paid OpenClaw/API turns were run

## Next
1. Bootstrap Debian/OpenClaw/phone-runtime on the AVD (reuse Phase 1/3 runbooks)
2. Install `scripts/phone-boot/` and run reboot proof
3. Screen-drive WebChat + agent tool proofs; save timestamped PNGs here
