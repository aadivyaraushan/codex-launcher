# iPhone native dependencies

Three generated inputs must exist under `ios/build/` before the Xcode project
will build. None of them is in Git — `**/build/` is ignored — and none is
fetched automatically, because all three are pinned artifacts obtained by hand.

Run [`bootstrap.sh`](bootstrap.sh) rather than following the steps by hand; it
calls the same two staging scripts and checks the same pins.

```sh
sh ios/Runtime/bootstrap.sh --check                       # what is present, what is missing
sh ios/Runtime/bootstrap.sh --pin /path/to/NodeMobile.zip # record a checksum
sh ios/Runtime/bootstrap.sh NODEMOBILE OPENCLAW WACLI     # build all three
```

`--check` and `--pin` work without Xcode. The full run does not: it needs the
iOS Simulator SDK, and `--check` says so plainly when it is absent.

## The three artifacts

| # | Artifact | Version | Output | Pinned by |
| --- | --- | --- | --- | --- |
| 1 | `NodeMobile.xcframework` | [nodejs-mobile v24.18.0-0](https://github.com/gmaclennan/nodejs-mobile/releases/tag/v24.18.0-0), asset `nodejs-mobile-ios-24.18.0-0.zip` — the **full** build, not lite | `ios/build/native-node/NodeMobile.xcframework` | **Nothing yet.** See below |
| 2 | `openclaw` package with dependencies installed | `2026.9.1` | `ios/build/native-node/runtime` | `package/stage.mjs`, on name and version |
| 3 | `wacli` source | [v0.17.1](https://github.com/openclaw/wacli/releases/tag/v0.17.1) — source, not a built executable | `ios/build/native-whatsapp` | `archive/build.sh`, on module, Go version, `go.mod` SHA-256 and source-tree SHA-256 |

## NodeMobile is not pinned yet

`expected_nodemobile_sha` in `bootstrap.sh` is the string `unpinned`, and until
it is a real hash the script refuses to stage the framework. That is deliberate:
this is a prerelease dependency, nobody has recorded a checksum for it, and
accepting an unverified binary silently is worse than stopping.

To close it, whoever holds the artifact runs:

```sh
sh ios/Runtime/bootstrap.sh --pin /path/to/nodejs-mobile-ios-24.18.0-0.zip
```

and records the result in two places — `expected_nodemobile_sha` in
`bootstrap.sh`, and the table above.

**Pin from an artifact you downloaded yourself.** A checksum that arrives
alongside the file it describes proves that the file is intact, not that it is
the right file. The archive and the extracted directory hash identically, so it
does not matter which one is used.

## Which tests a clean clone can actually run

Measured 2026-09-10 on a machine with no staged artifacts. This matters because
the handoff reports "Native Node tests: 22 passed, 0 failed", and that result is
not reproducible from a clone — half the files it names read staged output.

| Test file | Clean clone | Needs |
| --- | --- | --- |
| `tests/state/state.test.mjs` | passes (4) | nothing |
| `tests/state/bootstrap.test.mjs` | passes (1) | nothing |
| `tests/host/start.test.mjs` | passes (5) | nothing |
| `tests/package/stage.test.mjs` | **fails** | staged openclaw output |
| `compat/sqlite/tests/ownership.test.mjs` | **fails** | staged openclaw output |
| `compat/lifecycle/tests/export.test.mjs` | **fails** | staged openclaw output |

The three that fail read files out of `ios/build/runtime-recovery/openclaw-source/`
by content-hashed name, such as `run-CrJnbDWP.js`. Those hashes belong to one
particular openclaw build, so the tests are pinned to an artifact rather than to
the repository.

CI therefore runs only the hermetic three. The other three are a Mac step, after
the bootstrap has run. Making them hermetic — by staging a fixture rather than
reaching into `ios/build/` — is worthwhile and is not done.

## Notes

- Both staging scripts refuse to overwrite an existing output directory, and so
  does `bootstrap.sh`. Delete an output deliberately to rebuild it.
- The wacli archive is built for the **arm64 iOS Simulator**. Physical-device
  packaging is not proven.
- `bootstrap.sh` sets `GOTOOLCHAIN=go1.26.6` through the wacli script, so a
  different local Go is fetched rather than refused.
- Do not copy another machine's `ios/build/` or its runtime/session state as a
  shortcut. That is how the current single-machine dependency started.
