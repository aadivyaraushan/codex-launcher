# Task 14 public alpha release checkpoint

Date: 2026-07-14

## Purpose

Record the local evidence for the technical-alpha release files before any
repository is created, pushed, or published.

## Result

The repository now has local CI definitions, deterministic companion archives,
external Android signing configuration, public setup/security/compatibility
documents, dependency notices, and a manual-only release workflow. Nothing was
published and no hosted workflow was run.

The release workflow defaults to build-only. Creating a GitHub Release requires
the operator to set the manual `publish` input to `true`.

## Files and coverage

- `.github/workflows/android.yml`: Android unit, lint, and API 36 Pixel 9
  emulator tests.
- `.github/workflows/companion.yml`: Go tests and vet on macOS, Linux, and
  Windows, a separate race run, release-contract checks, repeatable package
  checks, and a high-severity vulnerability scan.
- `.github/workflows/release.yml`: manual signed APK/AAB and six-platform
  companion build, checksums, SPDX JSON SBOMs, SBOM vulnerability scans, GitHub
  attestations, artifact upload, and explicitly gated publication. Signing
  secrets exist only in the Gradle step, and the temporary key is deleted before
  third-party SBOM, scanning, or attestation steps run. The manual workflow
  reruns Go race/vet, repository contracts, the canonical protocol-schema gate,
  Android unit/lint, and Android 16 instrumentation before building.
  Publication creates or verifies a tag bound to the exact built commit and
  marks the GitHub Release as a prerelease.
- `release/companion-package/`: deterministic tar/zip packager and its test.
- `release/checks/public-alpha-release.test.mjs`: repository release contract.
- `docs/setup/android.md`: install, Tailscale, pairing, normal use, offline
  behavior, Android settings, and unpairing.
- `docs/setup/companion.md`: prerequisites, verification, install, setup,
  project selection, lifecycle commands, replacement, rollback, and uninstall.
- `docs/security/threat-model.md`: trust boundaries, local credential handling,
  signed pairing, replay defenses, stored data, limitations, and artifact
  verification.
- `docs/compatibility/codex.md`: verified and experimental platform/Codex paths.
- `THIRD_PARTY_NOTICES.md`: direct dependency license families and the release
  SBOM location.

Public-document coverage:

| Reader need | Document |
| --- | --- |
| What this alpha is and what is verified | `README.md` |
| Install and use the Android launcher | `docs/setup/android.md` |
| Install and operate the desktop companion | `docs/setup/companion.md` |
| Understand the security boundaries | `docs/security/threat-model.md` |
| Check operating-system and Codex support | `docs/compatibility/codex.md` |
| Inspect dependency licenses | `THIRD_PARTY_NOTICES.md` and generated SBOMs |

No `VERSION`, `CHANGELOG`, `CONTRIBUTING`, `ARCHITECTURE`, or repository TODO
document existed at this checkpoint, so no duplicate release metadata was
updated.

## Current official action versions

Every `uses:` reference is pinned to the exact commit behind its named official
tag:

| Action | Tag | Commit |
| --- | --- | --- |
| `actions/checkout` | v7 | `9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0` |
| `actions/setup-go` | v6 | `924ae3a1cded613372ab5595356fb5720e22ba16` |
| `actions/setup-java` | v5 | `0f481fcb613427c0f801b606911222b5b6f3083a` |
| `actions/upload-artifact` | v7 | `043fb46d1a93c77aae656e7c1c64a875d1fc6a0a` |
| `actions/download-artifact` | v8 | `3e5f45b2cfb9172054b4087a40e8e0b5a5461e7c` |
| `actions/attest` | v4 | `f6bf1532d7d6793fce74eac584813a8eee607999` |
| `gradle/actions/setup-gradle` | v6 | `90ddb51e90a5fd9ba75f40cf85156b7b41bf76a3` |
| `reactivecircus/android-emulator-runner` | v2 | `4c44018e59b437e86cdfc41da381398f93ed8808` |
| `anchore/sbom-action` | v0 | `e22c389904149dbc22b58101806040fa8d37a610` |
| `anchore/scan-action` | v7 | `e1165082ffb1fe366ebaf02d8526e7c4989ea9d2` |

The tags and required attestation permissions were checked against the current
official project documentation before writing the workflows.

## Red and green evidence

The release contract was written first. Its first run failed because
`.github/workflows/android.yml` did not exist. After implementation:

```text
public alpha release contract: 120 assertions passed
```

All repository Node checks passed:

```text
companion install contract: 18 assertions passed
design contract: 19 assertions passed
public alpha release contract: 120 assertions passed
repository baseline: 21 assertions passed
toolchain pins: 13 assertions passed
```

The full Go race suite and vet passed:

```text
go test ./... -race -count=1
go vet ./...
exit code: 0
```

Android unit tests and lint passed:

```text
BUILD SUCCESSFUL in 5s
33 actionable tasks
exit code: 0
```

The Android 16 Pixel 9 AVD instrumentation run completed with zero failures:

```text
Finished 85 tests on codex_launcher_pixel_9_api_36(AVD) - 16
1 externally driven network-loss test skipped
BUILD SUCCESSFUL in 2m 12s
```

The canonical schema gate also passed:

```text
validated 34 protocol frames and rejected 38 invalid frames
```

All three workflow files parsed as YAML, the release shell scripts passed Bash
syntax checks, and `git diff --check` passed.

## Android signing proof

A temporary test key was created outside the repository and removed after the
test. With all four signing environment variables present, Gradle built both
release formats:

```text
BUILD SUCCESSFUL in 1m 40s
57 actionable tasks
APK Signature Scheme v2: true
Signer: CN=Codex Launcher Test, O=Local Test, C=US
AAB: jar verified.
temporary-key Android release signing: PASS
```

No real signing key or signing password was written to the repository.

## Companion repeatability proof

The packager built darwin, linux, and windows archives for amd64 and arm64 twice
with the same fixed source time and test commit. Every output digest matched,
every archive checksum verified, and the embedded provenance described the
requested target and input commit. Its automated test also recalculates the
top-level archive/notice checksums, the embedded executable checksum, and every
provenance field:

```text
six-target companion reproducibility: PASS
```

The local comparison used the placeholder commit `0123456789abcdef` because
Task 14 was not yet committed.

After the implementation commit existed, the same full matrix was built twice
from commit `397e6222488d22bd60a92dac92ddc543300984fb` and source epoch
`1784019989`. Both output directories had identical digests and every entry in
both `SHA256SUMS` files verified. The first build produced:

```text
26a4dec6368ea44f871a2b868709b400bfca14c663ffb85929a7e6c797f69d6f  THIRD_PARTY_NOTICES.md
b98d039aa33da15643a4bef5e5efdb97c153ea280cc16f92960b9e0cdec7ac52  codex-launcher_0.1.0-alpha.1_darwin_amd64.tar.gz
e6fd142520af286f0e8df42a77682a335c9d791cd537785fd17ecbf3f7dbc134  codex-launcher_0.1.0-alpha.1_darwin_arm64.tar.gz
75be0415fd32d326dd345cc24591f84c0dbc8a66fb4eb3228f4f50d1a644455b  codex-launcher_0.1.0-alpha.1_linux_amd64.tar.gz
1e48202411048c3280bb945c816ece9a330305a31b2ebe4349c5c574dfc151cb  codex-launcher_0.1.0-alpha.1_linux_arm64.tar.gz
16260d47c4c218b339852f01e059dffc9f44f24d0291429390b1a93faf1e844f  codex-launcher_0.1.0-alpha.1_windows_amd64.zip
1f31b1d7a940ecab1b69037154e5efc9a62f965fb8adaef4ee0cedbb546d34e3  codex-launcher_0.1.0-alpha.1_windows_arm64.zip
six-target companion real-commit reproducibility: PASS
```

## Not yet verified

- GitHub Actions has not run these workflows from a clean hosted clone.
- SBOM generation, Grype vulnerability scanning, and GitHub attestation were not
  run locally; Syft/Grype were not installed and attestations require GitHub's
  hosted identity token.
- No release, repository, tag, push, or external account was created or used.
- No production Android signing key is configured.
- Native Linux and Windows service lifecycle tests remain experimental.
- Real Tailscale and the unlocked live ChatGPT Desktop follower flow remain
  untested in this environment.

## Independent review

A fresh release judge first defined its own standard, then reviewed the complete
uncommitted Task 14 tree. Its findings caused failing checks and fixes for shell
input injection, signing-secret lifetime, missing vulnerability scans, missing
notices, inaccurate Windows/Unix commands, exact-commit tag binding, prerelease
flags, mandatory test dependencies, checksum/provenance verification, and the
canonical protocol-schema gate.

The final recheck returned:

```text
READY
Zero P1/P2 findings remain.
```

The judge kept hosted workflow execution, real SBOM/scan/attestation, production
signing, native Linux/Windows lifecycle, real Tailscale, and the unlocked live
Desktop follower as external gaps rather than treating them as locally proven.

## Reuse

Run the local release contract with:

```sh
node release/checks/public-alpha-release.test.mjs
```

Build all companion archives with a real commit and fixed source time with:

```sh
SOURCE_DATE_EPOCH="$(git show -s --format=%ct HEAD)" \
go run ./release/companion-package \
  -output dist/companion \
  -version 0.1.0-alpha.1 \
  -commit "$(git rev-parse HEAD)" \
```

Then verify `dist/companion/SHA256SUMS` before using or publishing any artifact.
