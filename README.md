# Codex Launcher

Codex Launcher is an open-source technical alpha for using Codex tasks running
on your own computer from an Android home screen. The first release targets a
Pixel 9 on Android 16, with a companion program for macOS, Windows, and Linux.

The phone connects directly to the companion over the user's Tailscale network.
The computer remains fixed after pairing, while the approved project folder can
be changed above the prompt. When the computer cannot be reached, the launcher
says `Computer offline` and keeps All apps and Android Settings available.

The launcher does not copy ChatGPT authentication credentials, API keys, or
service credentials onto the phone. Codex and ChatGPT authentication remain on
the paired computer.

## Technical alpha status

The launcher, companion, private pairing, task transcript, actions, attachments,
foreground connection service, app drawer, and light/dark appearance are
implemented. Android behavior has automated coverage on a Pixel 9 Android 16
emulator. The macOS installer has completed a native install, replace, rollback,
and uninstall cycle.

This is still a technical alpha. The desktop archives are unsigned, the private
ChatGPT Desktop bridge can require updates when ChatGPT changes, and native
Windows/Linux lifecycle tests are not complete. Read the compatibility and
security guides before installing it on a computer with important work.

## Start here

- [Install the computer companion](docs/setup/companion.md)
- [Install and pair the Android launcher](docs/setup/android.md)
- [Security and privacy model](docs/security/threat-model.md)
- [Codex and platform compatibility](docs/compatibility/codex.md)
- [Approved implementation and test plan](planning/codex-launcher-v1-plan.md)

## Build checks

```bash
go test ./... -race -count=1
go vet ./...
./android/gradlew -p android testDebugUnitTest lintDebug
node release/checks/public-alpha-release.test.mjs
```

Android instrumentation requires an Android 16 emulator or device:

```bash
./android/gradlew -p android connectedDebugAndroidTest
```

## License

Apache License 2.0. See [LICENSE](LICENSE), [NOTICE](NOTICE), and
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
