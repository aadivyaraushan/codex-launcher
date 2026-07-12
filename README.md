# Codex Launcher

Codex Launcher is an open-source technical alpha for using Codex tasks running
on your own computer from an Android home screen. The first release targets a
Pixel 9 on Android 16, with a companion program for macOS, Windows, and Linux.

The phone connects directly to the companion over the user's Tailscale network.
The computer remains fixed after pairing, while the approved project/folder can
be changed above the prompt. When the computer cannot be reached, the launcher
says `Computer offline` and keeps All apps and Android Settings available.

The launcher does not copy ChatGPT authentication credentials, API keys, or
service credentials onto the phone. Codex and ChatGPT authentication remain on
the paired computer.

## Current state

This repository currently contains the approved product/design plan and early
build checks. It is not yet a usable launcher. Implementation progress and its
required tests are tracked in `planning/codex-launcher-v1-plan.md`.

## License

Apache License 2.0. See `LICENSE` and `NOTICE`.
