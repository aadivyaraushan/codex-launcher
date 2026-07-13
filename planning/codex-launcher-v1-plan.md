# Codex Launcher V1 Implementation Plan

> **Status:** In progress. Tasks 0-4, 2A, and 7 are complete. Task 5 now has the
> Go pairing core, exact host-SPKI pin, Android Keystore P-256 signing key,
> authenticated TLS 1.3 WebSocket server, and Android pinned pairing/session
> clients. The physical Pixel 9 hardware check, reduced-protection
> warning/enforcement, and packet capture remain. Task 6 now has strict Unix
> config storage, runtime wiring, CLI safety, queue, journal, and mobile
> transport cores. The transport binds proofs to their exact sockets, limits
> pre-authentication work, and requires `hello` before actions. Windows ACL
> storage, SQLite, setup, doctor, serving CLI entry
> point, and kill/restart integration remain. Task 7 is verified on a Pixel 9 Android 16 emulator:
> selectable Home role, offline Home, searchable app drawer, Android Settings
> escape, persisted appearance, bundled fonts, accessibility scans, and large
> text behavior. Task 8 now also has strict pairing-link parsing and a
> metadata-only paired-computer store. QR capture, pairing UI, and the existing
> folder/offline-state rules are not yet wired into the launcher UI.

**Goal:** Build an Apache-2.0 Android 16 home-screen launcher that lets a user pair their phone with their own macOS, Windows, or Linux computer over Tailscale and safely operate Codex tasks running on that computer.

**Architecture:** A native Kotlin/Compose launcher talks over a pinned-TLS WebSocket to a small Go companion service on the user's computer. Tailscale supplies private device-to-device routing. On macOS and Windows, the companion uses a version-checked adapter for ChatGPT Desktop's private local follower bridge so the phone can operate the same desktop-owned tasks. The public Codex app-server adapter remains available for CLI-owned and Linux tasks. ChatGPT/Codex authentication stays local, and neither raw desktop IPC nor app-server is exposed to the phone or tailnet.

**Primary target:** Pixel 9 on Android 16. Other Android devices are supported where the platform behavior is standard, but the Pixel 9 is the release device.

**Desktop targets:** macOS, Windows, and Linux. macOS gets native live verification first; Windows and Linux must pass hosted CI and VM smoke tests before their artifacts are called supported.

**License:** Apache License 2.0. Bundled Instrument Sans and JetBrains Mono retain their own OFL notices.

---

## One-minute system view

```text
┌──────────────── Pixel 9: native Android launcher ────────────────┐
│ Home / task transcript / approval / question / apps / settings  │
│ In-memory task view + Keystore signing key/draft + project ID    │
└───────────────────────────┬──────────────────────────────────────┘
                            │ pinned TLS WebSocket
                            │ private Tailscale route only
                            ▼
┌────────────── User computer: Go companion service ──────────────┐
│ Pair/revoke │ mobile API │ event journal │ durable prompt queue │
│ Desktop-version adapter │ redaction │ launch-at-login installer  │
└───────────────────────────┬──────────────────────────────────────┘
                            │ same-user local IPC only
                            ▼
┌────────────── ChatGPT Desktop task-owning window ────────────────┐
│ Existing tasks, live turns, approvals, questions, files, tools  │
└──────────────────────────────────────────────────────────────────┘

Linux / CLI-owned task: companion → public Codex app-server adapter
```

```text
DISCONNECTED → CONNECTING → SYNCING → ONLINE
      ▲             │           │        │
      └─────────────┴───────────┴────────┘
          timeout, VPN loss, host sleep, protocol failure

ONLINE → WAITING_FOR_APPROVAL / WAITING_FOR_ANSWER
ONLINE → WORKING → IDLE_AFTER_REPLY
Any state → INCOMPATIBLE_VERSION or REVOKED (no automatic retry)
```

`IDLE_AFTER_REPLY` means Codex finished one turn. The UI must not claim the overall task is complete.

## Locked product decisions

- Users install official Tailscale separately on their phone and computer and use their own tailnet. Users are never added to the developer's tailnet.
- V1 is open-source and self-hosted. It has no hosted relay, Firebase, analytics service, subscription, or paid infrastructure.
- A quiet Android foreground-service notification keeps the live connection available. If the user stops it, the launcher reconnects when opened and clearly shows stale/offline data until sync finishes.
- The Home composer always shows the paired computer and selected project/folder. The computer is fixed in V1; the project/folder is visibly tappable and changeable. Sending is disabled until a project is selected.
- The first prompt requires a project choice. Later prompts visibly reuse the last successful choice.
- ChatGPT/Codex auth and every third-party credential remain on the computer.
- The user approved ChatGPT Desktop's private local follower interface for V1.
  The companion pins supported desktop protocol versions, verifies the owning
  same-user ChatGPT process, and fails closed with
  `Desktop integration needs an update` when compatibility is not proven.
- Screens 8 and 9 stay removed. Attention is shown on Home; a finished Codex turn stays in the task transcript.
- One phone pairs with exactly one computer in V1. Changing computers means explicitly revoking the old pairing and pairing again; ordinary Home interaction changes only the project/folder.
- When the computer is unreachable, the launcher shows the approved `Computer offline` screen. It does not expose stale task lists or transcripts.
- The phone does not persist prompts, replies, commands, code, paths, or transcripts. It stores only the pairing material, a project identifier/display name, appearance settings, action state, and an encrypted unfinished draft. Event cursors are memory-only because a cold process has no rendered task state to resume.
- A persisted phone action record is metadata-only: action ID, action-kind enum, `PREPARED`/`SENT_UNKNOWN`/`CONFIRMED`, timestamps, non-content thread/turn IDs, payload SHA-256 digest, and non-sensitive result/error code. It never stores an action/result body, prompt, title, project/path, command, attachment name, or file metadata. Confirmed records are deleted after phone acknowledgement or 24 hours; keep at most 128 records, while unresolved `SENT_UNKNOWN` records remain until reconciled or explicitly dismissed.

## What already exists and will be reused

| Existing source | Reuse |
|---|---|
| `outputs/codex-launcher-visual-directions.html` | Approved seven-screen UI and light/dark behavior. |
| `DESIGN.md` | Tokens, state marks, accessibility, motion, and approval requirements. |
| `outputs/hermes-codex-android-reference.md` | Hermes parity target and staged scope. |
| Local Codex app-server schemas | Method names, notifications, approvals, and experimental question warning. Regenerate from the active CLI during implementation; do not trust the old snapshot blindly. |
| ChatGPT Desktop follower bridge | Primary macOS/Windows path for the actual desktop-owned tasks. Use only through a local version adapter; never forward raw frames. |
| Official Tailscale clients | Private routing, device identity, NAT traversal, MagicDNS. Do not embed or fork Tailscale. |
| Android platform launcher APIs | Home role, installed launcher activities, system settings intents, insets, accessibility, and foreground services. |

### Verified local constraints

- `/Applications/ChatGPT.app/Contents/Resources/codex --version` reports `codex-cli 0.144.0-alpha.4` on this Mac.
- The global npm `codex` command is currently broken because its native binary is missing. Companion setup must support an explicit Codex binary path and verify it by executing `--version`.
- The live CLI exposes `app-server daemon`, `proxy`, `generate-json-schema`, authenticated WebSocket modes, and stdio transport. V1 still uses a local companion boundary instead of publishing app-server on Tailscale.
- The refreshed stable schema includes `thread/list`, `thread/read`, `thread/start`, `thread/resume`, `thread/fork`, archive operations, `turn/start`, `turn/steer`, `turn/interrupt`, structured approvals, status/events, and `turn/completed`.
- `item/tool/requestUserInput` is still marked experimental. Native question cards require capability detection and a plain-message fallback.
- Go 1.26.5, Android SDK/API 36, Android Emulator 36.6.11, JDK 17.0.18,
  and the Pixel 9 Android 16 AVD are installed and passed the Task 1 bootstrap.
- ChatGPT Desktop package 26.707.51957 exposes a per-user local IPC router.
  A separate client loaded this desktop-owned task at revision 5774 and received
  version-11 live snapshots. An independent replay observed revisions 6050,
  6052, and 6074.
- The private bridge registers owner-routed start, steer, interrupt, approval,
  and requested-input actions. A harmless unknown approval ID proved routing;
  valid write behavior still requires the paid/account approval gate.

## Mobile contract

Use JSON over one pinned-TLS WebSocket. Keep the contract smaller and more stable than Codex app-server.

```json
{"v":1,"kind":"action","id":"01J...","name":"turn.start","body":{"threadId":"...","text":"..."}}
{"v":1,"kind":"event","seq":1842,"name":"turn.reply_finished","body":{"threadId":"...","turnId":"...","status":"completed"}}
{"v":1,"kind":"result","id":"01J...","ok":true,"body":{}}
```

Rules:

- `id` identifies one phone action, but does not by itself make a downstream Codex action safe to retry.
- Action state is durable: `PREPARED` before any Codex write, `SENT_UNKNOWN` immediately before the write crosses the process boundary, and `CONFIRMED` only after a Codex response or deterministic reconciliation is stored.
- A retry of `PREPARED` may send. A retry of `CONFIRMED` replays the stored result. A retry of `SENT_UNKNOWN` must reconcile against Codex state using behavior proven in Task 2; if reconciliation cannot prove the outcome, the phone shows `Outcome unknown` and requires the user to inspect/retry deliberately.
- Crash tests inject failure before send, after send/before response, after response/before durable commit, and after commit/before phone receipt for every side-effecting action class.
- `seq` is a cumulative companion event cursor. During a warm reconnect, the phone updates its in-memory rendered state first, then sends `event.ack` for the highest contiguous sequence and may resume from that in-memory cursor. After process death, app restart, explicit offline-content clear, or any state uncertainty, the phone declares `no_local_state` and requires a full snapshot/thread read regardless of any prior transport acknowledgement. A snapshot replaces in-memory state atomically and declares the new base sequence.
- Approval and question replies are never placed in the normal prompt queue. They route immediately to their exact request ID or fail closed as expired.
- All unknown event fields are ignored; unknown event names produce an explicit compatibility warning and are preserved in diagnostic logs without secrets.
- Android notifications use generic state text only, such as `Codex replied`, `Codex needs approval`, or `Codex needs your answer`. They contain no task name, prompt, command, file content, path, or credential because Android may retain notification text outside the app.

| Crash point | Required outcome |
|---|---|
| Before Codex send | Safe automatic retry. |
| After send, before response | Reconcile by proven thread/turn state or show `Outcome unknown`; never blind retry. |
| After response, before companion commit | Reconcile or show `Outcome unknown`; never infer success from timeout. |
| After companion commit, before phone receipt | Replay the committed result and events by action ID/cursor. |

## Repository shape

Purpose-based folders keep each directory to roughly two or three files.

```text
android/
  app/                         Android entry point and manifest
  app/src/main/kotlin/app/codexlauncher/
    launcher/home/             Home screen + view model
    launcher/apps/             app drawer + installed-app source
    task/transcript/           task log + activity rows
    task/control/              queue / redirect / stop
    decision/approval/         security sheet
    decision/question/         native question + fallback
    connection/pairing/        QR/manual pairing
    connection/stream/         WebSocket foreground service
    connection/state/          reconnect state machine
    project/selection/         approved-root folder chooser
    storage/projects/          opaque project ID/display preference
    storage/drafts/            encrypted unfinished draft only
    storage/actions/           bounded metadata-only action records
    storage/wipe/              atomic local-state removal on unpair
    storage/secrets/           Android Keystore device signing-key storage
    appearance/theme/          Quiet Instrument tokens

companion/
  cmd/codex-launcher/          CLI entry point
  internal/codex/probe/        version discovery and feasibility spike
  internal/codex/desktopipc/   private desktop follower adapter
  internal/codex/appserver/    public CLI/Linux app-server adapter
  internal/codex/taskstate/    stable task-state mapping
  internal/mobileapi/contract/ mobile contract validation
  internal/mobileapi/transport/ TLS WebSocket server/session
  internal/pairing/            one-time pairing and revocation
  internal/eventjournal/       bounded companion event history
  internal/promptqueue/        durable queued prompts
  internal/decisions/          request routing and safe approval display
  internal/hostinstall/        launchd/systemd/Task Scheduler

protocol/
  schema/                      three JSON Schemas: envelope/action/event
  fixtures/                    cross-language golden messages and crash cases

release/
  checks/                      release smoke scripts
  packaging/                   archive and checksum definitions

.github/workflows/             Android, companion, and release CI
```

## Implementation sequence

Every behavior task follows the fixed order: write the observable test, run it and capture the expected failure, implement the smallest code, then rerun at unit and integration level.

### Task 0: Establish a safe repository baseline and worktree

**Objective:** Create the required isolated worktree before making any new configuration, design, or source edit.

**Files:**

- Create: `.gitignore`
- Create: `LICENSE`
- Create: `NOTICE`
- Create: `README.md`
- Create: `release/checks/design-contract.test.mjs`
- Modify: `outputs/codex-launcher-visual-directions.html`
- Modify: `DESIGN.md`

**Steps:**

1. Ask for explicit permission to stage only the existing approved design/research artifacts and this plan. Do not stage `.gstack/` or `work/`.
2. Create the initial baseline commit without editing project files, then immediately create and enter a dedicated implementation worktree.
3. In the worktree, add ignores for `.gstack/`, `work/`, Android/Gradle output, Go binaries, IDE files, VM disks, signing keys, certificates, tokens, and local configuration.
4. Add Apache-2.0 and font notices. Do not add API keys, Tailscale credentials, signing material, or ChatGPT state.
5. Write static design assertions for the fixed computer label, changeable project, disabled first send, `Replied` language, task overflow, new-task options, dictation, seven-screen count, and absence of screens 8/9.
6. Run the assertions against the current sources and capture the expected failures.
7. Update the Home visual and `DESIGN.md` with a fixed computer label, visibly tappable project/folder, and disabled-send first-use behavior.
8. Reconcile task-state language in `DESIGN.md`, the HTML, and `outputs/hermes-codex-android-reference.md`: replace semantic `Completed` task claims with `Replied`/idle-after-reply unless a future explicit goal contract proves completion.
9. Add visual examples for task overflow actions, new-task model/reasoning/permission choices, and Android dictation without adding standalone screens 8 or 9. Get user approval on this small visual round before implementing those controls.
10. Rerun the same assertions green, proving seven screens remain, screen 8/9 stay absent, the project selector appears in both modes, and no ordinary `turn/completed` state is labelled `Task completed` or `Completed`.

**Verify:** The implementation branch is in a separate worktree before step 3; `git status --short` lists only intentional files; `git ls-files` contains no `.gstack/`, `work/`, key, token, certificate, or VM image.

### Task 1: Bootstrap reproducible Go and Android builds

**Objective:** Make the first Go test and Android unit/instrumentation target runnable from a clean clone before feature work begins.

**Files:**

- Create: `go.mod`
- Generate: `go.sum`
- Create: `android/settings.gradle.kts`
- Create: `android/build.gradle.kts`
- Create: `android/gradle/libs.versions.toml`
- Create: `android/gradle/wrapper/gradle-wrapper.properties`
- Generate: `android/gradlew`, `android/gradlew.bat`, and wrapper JAR
- Create: `android/app/build.gradle.kts`
- Create: `android/app/src/main/AndroidManifest.xml`
- Create: `android/app/src/main/kotlin/app/codexlauncher/LauncherActivity.kt`
- Create: `android/app/src/main/kotlin/app/codexlauncher/diagnostics/AppLog.kt`
- Create: `android/app/src/test/kotlin/app/codexlauncher/diagnostics/AppLogTest.kt`
- Create: `release/checks/bootstrap-smoke.sh`

**Red tests:** Write `bootstrap-smoke.sh` first. It runs `go test ./...`, `./android/gradlew :app:testDebugUnitTest`, and `./android/gradlew :app:connectedDebugAndroidTest`; capture the expected missing-module/wrapper/toolchain failures. Write logger tests that fail until secrets, prompts, commands, paths, tokens, and large values are redacted while feature tag, IDs, input/output shapes, branch reasons, and contextual errors remain.

**Implementation:** Re-check current official Go, Android Gradle Plugin, Kotlin, Compose BOM, Gradle, Android SDK, and JDK compatibility documentation before pinning versions. Use the standard Gradle layout as the explicit exception to the two-to-three-files-per-folder preference. Add the smallest empty Go package, Android activity, unit test, and instrumentation smoke needed to prove both toolchains. Add the structured Android logger/redaction wrapper before the lifecycle spike or any feature code. Installation of missing toolchains still requires the pre-run blocker approval.

**Verify:** From a clean clone with documented toolchains, `go test ./...`, `./android/gradlew :app:testDebugUnitTest`, and one emulator/Pixel `connectedDebugAndroidTest` are green before Task 2 begins.

### Task 2: Prove the Codex and Android 16 integration boundaries before building UI

**Objective:** Determine which Codex tasks the companion can control and whether Android 16 permits the proposed always-connected launcher lifecycle.

**Files:**

- Create: `companion/internal/codex/probe/probe_test.go`
- Create: `companion/internal/codex/probe/probe.go`
- Create: `android/app/src/androidTest/kotlin/app/codexlauncher/connection/ConnectionLifecycleSpikeTest.kt`
- Create: `android/app/src/main/kotlin/app/codexlauncher/connection/ConnectionLifecycleSpike.kt`
- Create: `saved-results/codex-app-server-compatibility.md`
- Create: `saved-results/android-16-connection-lifecycle.md`

**Red tests:** A fake JSON-RPC server verifies initialize → list/read → subscribe/resume → event → approval response ordering and rejects out-of-order messages. A Pixel 9 instrumentation spike fails until foreground-service startup, notification, WebSocket survival, and recovery states are observed under Android 16.

**Implementation:**

1. Discover the Codex binary from explicit config first, then `PATH`; validate with `--version`.
2. Generate schemas into a temporary directory and record the CLI version/capabilities.
3. Compare `codex app-server proxy`, a dedicated stdio child, and ChatGPT
   Desktop's same-user local follower bridge.
4. Verify list/read without starting a model turn.
5. Before the first real model-backed turn, identify the active ChatGPT account/project and estimated call count, then obtain explicit user approval under the money rule.
6. Test whether a desktop-started active turn is observable and controllable
   from the companion. Record read, routing, and valid-write evidence separately
   so a harmless no-op is never presented as proof of a real write.
7. Re-read the current official Android foreground-service documentation at implementation time and record the target-SDK requirements used.
8. On Pixel 9, test default-launcher cold start, reboot/user unlock, screen off, Doze, Wi-Fi↔5G, Tailscale stop/restart/update, notification denial/channel removal, foreground-service start rejection, OS process kill, user service stop, and force-stop recovery.

**Hard gates:** Desktop reads and harmless owner routing passed through the
private follower bridge. Valid start/steer/interrupt/approval behavior remains
gated on a controlled live test after account/cost approval. Do not silently
fall back to companion-owned tasks. If Android 16 does not permit the
connected-device foreground-service design for this use, stop and revisit
notification/background architecture before building the launcher around it.

**Verify:** `go test ./companion/internal/codex/probe -race`; Pixel instrumentation evidence; both reports contain versions, commands, platform state, observed results, and explicit pass/fail gates.

### Task 2A: Freeze the private desktop follower bridge before using it

**Objective:** Turn the verified macOS follower path into a narrow,
version-checked Go adapter that cannot send unknown actions after a ChatGPT
Desktop update.

**Files:**

- Create: `companion/internal/codex/desktopipc/protocol.go`
- Create: `companion/internal/codex/desktopipc/client.go`
- Create: `companion/internal/codex/desktopipc/client_test.go`
- Create: `companion/internal/codex/desktopipc/testdata/snapshot.json`
- Create: `companion/internal/codex/desktopipc/testdata/invalid-frames.jsonl`
- Update: `saved-results/codex-app-server-compatibility.md`

**Observable guarantees:** A same-user local client discovers the desktop
endpoint, completes `initialize`, accepts only pinned message versions, retains
a desktop-owned task's full snapshot plus ordered deltas, and routes only
an allow-listed action to the window that owns that task. An unknown desktop
build, unknown message version, missing owner, malformed/oversized frame,
disconnect, or uncertain write produces a typed error and no automatic retry.

**Red tests:** Use a fake framed IPC router and captured redacted fixtures to
cover partial length headers, partial bodies, zero/oversized lengths, malformed
JSON, mismatched request IDs, router discovery, owner unavailable, version 0 or
unknown versions, snapshot then ordered delta, duplicate/out-of-order revision,
disconnect/reconnect, harmless approval-route probe, and every allow-listed
write action's exact parameter shape. Confirm the new package is missing and
the tests fail for that reason before production code is added. Commit this RED
checkpoint separately.

**Implementation:** Use the observed four-byte little-endian length plus JSON
framing. Discover the macOS endpoint beneath `os.TempDir()` rather than storing
its temporary path. Verify the socket and ChatGPT process belong to the current
OS user. Pin ChatGPT Desktop package 26.707.51957's observed versions:
`thread-stream-state-changed` 11; start/load/compact/steer/settings/approval/
input actions 1; interrupt and edit-last-turn 2. Task 2A's executable action
allowlist is start, steer, interrupt, and command approval; each has an exported
call path and a pinned success shape. Task 4 adds compact, settings, file and
permission approvals, requested input, MCP elicitation, and edit-last-turn with
their own typed request and response contracts before exposing them to Android.
Parse unknown fields but reject unknown methods or incompatible versions. Use structured `slog` records tagged
`[desktop-ipc]` with message kind, version, task ID, revision, and branch reason;
never log task content, prompts, commands, paths, approval text, or credentials.

**Live compatibility check:** Without starting a model call, initialize against
the live socket, load the current desktop-owned task, require a full snapshot,
and route a fresh nonexistent approval ID that is guaranteed to be a no-op.
This proves connection and routing only. A real start/steer/interrupt/approval
test remains blocked until the money rule is cleared.

**Verify:** `go test ./companion/internal/codex/desktopipc -race -cover` is green
with at least 80% statement coverage; the live harmless compatibility test is
green on macOS; `git diff --check` is clean; the saved report keeps the valid
write gap explicit.

### Task 3: Freeze the narrow mobile protocol with shared fixtures

**Objective:** Prevent the Android and Go implementations from drifting.

**Files:**

- Create: `protocol/schema/envelope.schema.json`
- Create: `protocol/schema/action.schema.json`
- Create: `protocol/schema/event.schema.json`
- Create: `protocol/fixtures/session.jsonl`
- Create: `protocol/fixtures/approval.jsonl`
- Create: `protocol/fixtures/reconnect.jsonl`
- Create: `companion/internal/mobileapi/contract/messages.go`
- Create: `companion/internal/mobileapi/contract/validation.go`
- Create: `companion/internal/mobileapi/contract/contract_test.go`
- Create: `android/app/src/main/kotlin/app/codexlauncher/connection/protocol/ProtocolMessage.kt`
- Create: `android/app/src/main/kotlin/app/codexlauncher/connection/protocol/ProtocolCodec.kt`
- Create: `android/app/src/test/kotlin/app/codexlauncher/connection/ProtocolContractTest.kt`

**Red tests:** Go and Kotlin contract tests both reject duplicate action IDs, missing/gapped acknowledgements, cold-start cursor resume without local state, unsupported major versions, malformed approvals, unsafe project paths, corrupted attachment chunks, and oversized frames. Crash fixtures cover every action state transition. A killed-phone fixture proves `no_local_state` receives a full snapshot/thread read even if a prior acknowledgement was sent.

**Implementation:** Define version negotiation, snapshot/event/result/ack envelopes, cumulative acknowledgement and snapshot base-sequence rules, action-state reconciliation, capability flags, 256 KiB JSON-frame limit, and typed error codes. Define authenticated binary attachment frames with upload ID, ordered chunk number, declared total size, server-advertised size limit, SHA-256 digest, cancellation, resume policy, and completion acknowledgement. Server-advertised configurable defaults are 20 MiB per file, two concurrent uploads per device, four globally, 100 MiB total temporary storage, and 15-minute expiry; reject before allocation when any quota would be exceeded. Keep raw Codex payloads out of the public contract.

**Verify:** The production Go contract types/validator and production Kotlin messages/codec parse every golden fixture and reject every invalid fixture; tests do not contain a second test-only parser. The schema check runs in the local release gate now; Task 14 wires that same gate into hosted CI after the public repository and its billing owner are chosen.

### Task 4: Build the companion's task adapters and safe state mapping

**Objective:** Convert current app-server methods/events into stable launcher concepts without inventing task completion.

**Files:**

- Create: `companion/internal/codex/appserver/client.go`
- Create: `companion/internal/codex/appserver/client_test.go`
- Create: `companion/internal/codex/taskstate/mapper.go`
- Create: `companion/internal/codex/taskstate/mapper_test.go`

**Red tests:** Cover working, waiting for approval, waiting for answer, failed, interrupted, and idle-after-reply states; verify `turn/completed` never becomes semantic task completion.

**Implementation:** Use `desktopipc` as the primary macOS/Windows source for
desktop-owned tasks and `appserver` for CLI/Linux tasks. Both adapters feed one
stable task-state mapper. Implement initialize, task list/read/start/resume/
fork/archive/name, model list, reasoning/permission inputs, turn start/steer/
interrupt, diff/item streams, approvals, permissions, and capability-gated
questions. Preserve unknown items as safe generic activity entries. Never
silently move a desktop-owned task into a separate app-server runtime.

**Logging:** Use Go's structured `slog` with `[codex-adapter]`, thread/turn/item IDs, branch decisions, output counts, and contextual errors. Never log prompts, command bodies, file contents, auth tokens, or pairing secrets.

**Verify:** `go test ./companion/internal/codex/... -race`; replay captured
redacted fixtures from the pinned desktop bridge and public app-server schemas.

### Task 5: Add pairing, pinned TLS, revocation, and replay protection

**Objective:** Ensure a tailnet device cannot operate Codex merely because it can reach the computer.

**Files:**

- Create: `companion/internal/pairing/service.go`
- Create: `companion/internal/pairing/store.go`
- Create: `companion/internal/pairing/service_test.go`
- Create: `android/app/src/main/kotlin/app/codexlauncher/storage/secrets/PairingKeyStore.kt`
- Test: `android/app/src/androidTest/kotlin/app/codexlauncher/storage/secrets/PairingKeyStoreTest.kt`

**Red tests:** Expired, reused, guessed, revoked, wrong-identity, wrong-device-key, wrong-host, wrong-version, stale-session, replayed signature, and substituted-key pairing/auth attempts fail; disconnect during pairing leaves no half-paired device; revocation closes an existing socket immediately; signing-key rotation overlap, lost-phone recovery, lost-computer recovery, and restored-backup behavior are explicit. Pixel instrumentation fails until private-key export is impossible and `KeyInfo` reports hardware-backed protection; missing/exportable protection blocks pairing, while a non-Pixel software-backed but non-exportable key produces the stated reduced-protection warning.

**Implementation:** Generate a stable local Ed25519 host identity and renewable TLS certificate using that identity key. The phone pins the identity's encoded SubjectPublicKeyInfo rather than an expiring leaf certificate. The phone generates a P-256 ECDSA signing key inside Android Keystore because Android KeyMint guarantees hardware-backed ECDSA but does not expose hardware-backed Ed25519 generation. A five-minute single-use 128-bit pairing secret and QR URI bind host address, port, host identity key, and protocol major version; successful enrollment sends the phone's device public key, and the companion stores only that public key plus device metadata. Each WebSocket receives a fresh server nonce and authenticates by signing nonce + host identity + protocol version + session ID. Rotate a device key by signing the new public key with the current private key over an authenticated session; keep both public keys only until the phone confirms the new key, then remove the old key. Revocation removes all keys for the device and closes matching sessions immediately.

**Recovery:** Lost phone → revoke from companion CLI. Lost companion identity/database or untrusted backup restore → invalidate all pairings and pair again. Certificate renewal with the same host identity pin is automatic; host identity-key change always requires explicit re-pairing. Lost Android signing key also requires pairing again. No “accept new certificate/key” bypass is offered.

**Security:** Bind only to an address reported by the local Tailscale client. Refuse wildcard/public binds by default. Store the desktop identity and paired-device public-key database in the current user's protected config directory; installer tests verify owner-only POSIX modes and Windows ACL inheritance. Pairing and approval errors fail closed.

**Verify:** Go security tests with `-race`; Android Keystore instrumentation on Pixel 9 proves the private key is non-exportable and reports hardware security level through `KeyInfo`. Pixel 9 release support requires hardware-backed protection. Another device with a non-exportable but software-backed Keystore may continue only after a visible reduced-protection warning; missing/exportable key protection fails pairing. Packet capture confirms no plaintext application data on the local network.

### Task 6: Compose a runnable companion with durable reconnect and prompt queue

**Objective:** Produce the first runnable companion CLI and survive Wi-Fi/mobile transitions, process restarts, duplicate frames, and temporary computer loss without duplicate Codex actions.

**Files:**

- Create: `companion/internal/eventjournal/store.go`
- Create: `companion/internal/eventjournal/store_test.go`
- Create: `companion/internal/promptqueue/queue.go`
- Create: `companion/internal/promptqueue/worker.go`
- Create: `companion/internal/promptqueue/queue_test.go`
- Create: `companion/internal/mobileapi/transport/server.go`
- Create: `companion/internal/mobileapi/transport/session.go`
- Create: `companion/internal/mobileapi/transport/server_test.go`
- Create: `companion/internal/app/app.go`
- Create: `companion/internal/app/config.go`
- Create: `companion/internal/app/app_test.go`
- Create: `companion/internal/cli/root.go`
- Create: `companion/internal/cli/commands.go`
- Create: `companion/internal/cli/cli_test.go`
- Create: `companion/cmd/codex-launcher/main.go`

**Red tests:** Cover startup/config validation, component wiring, `setup/pair/devices/revoke/status/doctor/version`, cumulative warm cursor acknowledgement, cold `no_local_state` snapshot after phone-process death, compacted cursor → atomic snapshot/base sequence, duplicate confirmed-action replay, every crash point in the action table, unknown-outcome recovery, queued prompt order, adapter restart, thread deleted while queued, redirect-not-supported, and approval bypass of the normal queue.

**Implementation:** Compose the Codex runtime, pairing service, mobile transport, event journal, prompt queue, and approved-project configuration behind one CLI. Use SQLite in the user's config directory for paired-device public-key records, bounded event journal, action results, and queued prompts. Codex remains the source of truth for threads; do not copy full transcripts into the companion database. Task 13 later adds background-service installation and binary replacement to this already working CLI.

**Verify:** `go test ./companion/internal/app ./companion/internal/cli ./companion/internal/mobileapi/transport ./companion/internal/eventjournal ./companion/internal/promptqueue -race`; run `go run ./companion/cmd/codex-launcher doctor`; kill/restart integration proves safe replay where confirmed and a visible `Outcome unknown` where Codex cannot be reconciled. It must never claim impossible exactly-once execution.

### Task 7: Create the Android launcher shell and approved design system

**Objective:** Become a selectable Android Home app while keeping Android Settings and all apps reachable offline.

**Files:**

- Modify: `android/app/src/main/AndroidManifest.xml`
- Modify: `android/app/src/main/kotlin/app/codexlauncher/LauncherActivity.kt`
- Create: `android/app/src/main/kotlin/app/codexlauncher/LauncherApplication.kt`
- Create: `android/app/src/main/kotlin/app/codexlauncher/appearance/theme/QuietInstrumentTheme.kt`
- Create: `android/app/src/main/kotlin/app/codexlauncher/appearance/theme/QuietInstrumentTokens.kt`
- Create: `android/app/src/main/kotlin/app/codexlauncher/appearance/theme/ThemePreferenceStore.kt`
- Test: `android/app/src/test/kotlin/app/codexlauncher/appearance/theme/ThemePreferenceStoreTest.kt`
- Test: `android/app/src/androidTest/kotlin/app/codexlauncher/LauncherRoleTest.kt`

**Red tests:** HOME intent resolves, Settings escape works offline, system back never traps the user, status/gesture/keyboard insets remain visible, and large font/reduced motion preserve controls.

**Implementation:** Kotlin + Jetpack Compose, one Android app module with purpose-based packages, Hilt only if constructor wiring becomes repetitive, and no speculative feature modules. Bundle approved fonts and tokens; implement Follow system/Light/Dark with DataStore.

**Verify:** Gradle unit tests, Compose UI tests, accessibility scanner, and manual Pixel 9 default-launcher switch/recovery.

### Task 8: Implement pairing, project selection, and offline truthfulness

**Objective:** Make the fixed paired computer, changeable execution folder, and connection state obvious before the first prompt.

**Files:**

- Create: `android/app/src/main/kotlin/app/codexlauncher/connection/pairing/PairingScreen.kt`
- Create: `android/app/src/main/kotlin/app/codexlauncher/connection/pairing/PairingViewModel.kt`
- Create: `android/app/src/main/kotlin/app/codexlauncher/project/selection/ProjectSelector.kt`
- Create: `android/app/src/main/kotlin/app/codexlauncher/project/selection/ProjectSelectionViewModel.kt`
- Create: `android/app/src/main/kotlin/app/codexlauncher/storage/projects/ProjectSelectionStore.kt`
- Test: `android/app/src/test/kotlin/app/codexlauncher/storage/projects/ProjectSelectionStoreTest.kt`
- Create: `android/app/src/main/kotlin/app/codexlauncher/connection/state/ConnectionStateMachine.kt`
- Create: `companion/internal/projects/service.go`
- Create: `companion/internal/projects/pathrules.go`
- Create: `companion/internal/projects/service_test.go`
- Modify: `companion/internal/app/app.go`
- Modify: `companion/internal/app/config.go`
- Modify: `companion/internal/cli/commands.go`
- Create: `companion/integration/projects/projects_test.go`
- Test: matching unit and Compose UI tests under `android/app/src/test/` and `android/app/src/androidTest/`.

**Red tests:** First prompt cannot send without a project; the computer label is not a host switcher; tapping the project changes only the folder; removed/unavailable folders require a new choice; `..`, symlink/junction escape, volume changes, case differences, renamed roots, and Windows path forms cannot escape approved roots; offline always shows `Computer offline`; revoked and incompatible states do not retry forever.

**Implementation:** CameraX + ZXing Core for QR scanning without Firebase, plus manual URI entry. Companion setup defines approved root aliases. The phone receives opaque project IDs and display names, not unrestricted paths. The companion resolves canonical paths/symlinks, verifies volume and containment with OS-aware rules, and never exposes a general remote filesystem browser.

**Verify:** `go test ./companion/internal/projects ./companion/internal/app ./companion/integration/projects -race`; Pair/unpair/re-pair on Pixel 9 over home Wi-Fi and mobile data; change among approved folders; turn off Tailscale and the computer to verify the same truthful `Computer offline` surface with specific recovery detail.

### Task 9: Build Home, task transcript, task management, and new-task controls

**Objective:** Match approved screens 1 and 2 using real task data while covering the reference V1 controls without persisting work contents.

**Files:**

- Create: `android/app/src/main/kotlin/app/codexlauncher/launcher/home/HomeScreen.kt`
- Create: `android/app/src/main/kotlin/app/codexlauncher/launcher/home/HomeViewModel.kt`
- Create: `android/app/src/main/kotlin/app/codexlauncher/task/transcript/TaskScreen.kt`
- Create: `android/app/src/main/kotlin/app/codexlauncher/task/transcript/TranscriptMapper.kt`
- Create: `android/app/src/main/kotlin/app/codexlauncher/task/management/TaskActionsSheet.kt`
- Create: `android/app/src/main/kotlin/app/codexlauncher/task/management/TaskActionsViewModel.kt`
- Create: `android/app/src/main/kotlin/app/codexlauncher/task/configuration/NewTaskOptionsSheet.kt`
- Create: `android/app/src/main/kotlin/app/codexlauncher/task/configuration/NewTaskOptionsViewModel.kt`
- Create: `android/app/src/main/kotlin/app/codexlauncher/storage/drafts/EncryptedDraftStore.kt`
- Create: `android/app/src/main/kotlin/app/codexlauncher/storage/drafts/DraftKeyStore.kt`
- Create: `android/app/src/main/kotlin/app/codexlauncher/storage/actions/ActionRecord.kt`
- Create: `android/app/src/main/kotlin/app/codexlauncher/storage/actions/ActionRecordStore.kt`
- Create: `android/app/src/main/kotlin/app/codexlauncher/storage/wipe/LocalStateWiper.kt`
- Test: `android/app/src/test/kotlin/app/codexlauncher/storage/actions/ActionRecordStoreTest.kt`
- Test: `android/app/src/androidTest/kotlin/app/codexlauncher/storage/drafts/EncryptedDraftStoreTest.kt`
- Test: `android/app/src/androidTest/kotlin/app/codexlauncher/storage/wipe/LocalStateWiperTest.kt`

**Red tests:** Empty, one, many, long-title, rapid-event, unknown-item, failed, interrupted, waiting, working, replied/idle, rename, archive, fork, resume, invalid/stale task, model capability changes, reasoning/permission choices, and process recreation. Offline/process restart shows no persisted transcript content. On-device data inspection for `PREPARED`, `SENT_UNKNOWN`, and `CONFIRMED` proves only the locked metadata allowlist is stored, record caps/expiry apply, and no action/result body or work content appears. Draft/action tests cover save/load, process death, expiry, cap eviction, unresolved retention, partial/failed write, corrupt ciphertext/record, backup exclusion, and atomic wipe after unpair.

**Implementation:** Task lists and transcripts live in memory and are rebuilt from the companion after sync. Persist no task titles, prompts, replies, commands, code, paths, rendered transcript items, or event cursor. Store only the selected opaque project ID/display name, the locked metadata-only phone-action record, and an encrypted unfinished draft. The draft uses its own non-exportable Android Keystore AES-GCM key, never the P-256 pairing key; writes use temporary file + fsync + atomic replace. Action records use a dedicated metadata-only serializer that cannot encode action/result bodies and applies the locked cap/retention rules. `LocalStateWiper` removes project selection, draft ciphertext/key, action records, and pairing key as one fail-closed unpair operation, retrying incomplete deletion before allowing a new pairing. Disable Android backup for app-private state, disable recents screenshots, and use `FLAG_SECURE` while approval/question surfaces are visible. Tool activity collapses by default; diffs and command output open dedicated viewers. Task overflow owns rename/archive/fork; the new-task sheet owns model, reasoning, and permission mode using only host-advertised options.

**Verify:** Mapper/unit/Compose tests, on-device data-directory inspection, Android backup exclusion check, wipe-on-unpair test, recents/sensitive-screen check, both-theme screenshots, and real UI comparison on Pixel 9.

### Task 10: Implement send, durable queue, redirect, stop, dictation, and attachments

**Objective:** Make busy-task controls explicit and safe.

**Files:**

- Create: `android/app/src/main/kotlin/app/codexlauncher/task/control/TaskControls.kt`
- Create: `android/app/src/main/kotlin/app/codexlauncher/task/control/TaskControlViewModel.kt`
- Create: `android/app/src/main/kotlin/app/codexlauncher/task/control/AttachmentPicker.kt`
- Create: `android/app/src/main/kotlin/app/codexlauncher/task/control/PromptDictation.kt`
- Create: `companion/internal/attachments/store.go`
- Create: `companion/internal/attachments/quota.go`
- Create: `companion/internal/attachments/store_test.go`
- Modify: `companion/internal/mobileapi/transport/server.go`
- Modify: `companion/internal/mobileapi/transport/session.go`
- Modify: `companion/internal/app/app.go`
- Modify: `companion/internal/app/config.go`
- Create: `companion/integration/attachments/attachments_test.go`
- Test: matching unit, integration, and Compose UI tests.

**Red tests:** Idle send, busy queue, supported redirect, unsupported redirect fallback, stop confirmation, double-tap, stale task, offline draft, queue reorder rejection, recognizer absent/cancel/error/success, attachment too large, per-device/global concurrency limit, total-temp quota, disk full, expired upload, corrupt hash, missing/out-of-order chunk, upload interruption/resume/cancel, and companion restart cleanup.

**Implementation:** The composer labels the chosen behavior before sending. The companion owns durable queue order; `turn/steer` is used only when the current Codex capability says it is accepted. The voice control invokes Android's installed speech recognizer, returns editable text to the composer, and sends no audio to the companion. Android system photo/document pickers provide attachments. Uploads use the Task 3 binary contract, a host-advertised size limit, SHA-256 verification, owner-only random temporary files, cleanup on cancel/failure/startup expiry, and handoff to Codex only after completion. V1 excludes video and live audio.

**Verify:** `go test ./companion/internal/attachments ./companion/internal/mobileapi/transport ./companion/integration/attachments -race`; Android end-to-end fake-server tests plus approved live tests after account/cost confirmation.

### Task 11: Implement approvals and questions as immediate decision paths

**Objective:** Match approved screens 3 and 5 without allowing disconnects or ambiguous replies to grant access.

**Files:**

- Create: `android/app/src/main/kotlin/app/codexlauncher/decision/approval/ApprovalSheet.kt`
- Create: `android/app/src/main/kotlin/app/codexlauncher/decision/approval/ApprovalViewModel.kt`
- Create: `android/app/src/main/kotlin/app/codexlauncher/decision/question/QuestionSheet.kt`
- Create: `android/app/src/main/kotlin/app/codexlauncher/decision/question/QuestionFallback.kt`
- Create: `companion/internal/decisions/router.go`
- Create: `companion/internal/decisions/redaction.go`
- Create: `companion/internal/decisions/router_test.go`
- Modify: `companion/internal/mobileapi/transport/session.go`
- Modify: `companion/internal/app/app.go`
- Create: `companion/integration/decisions/decisions_test.go`

**Red tests:** Allow once, offered longer scope, deny, timeout, disconnect, duplicate answer, expired request, concurrent approval/question FIFO, cross-request ID substitution, `Not now`, secret question, partially/fully redacted command, experimental capability missing, and plain-message fallback. Each test asserts the exact app-server response or deliberate absence of a response.

**Implementation:** Render only scopes Codex actually offers. Show computer, project, working directory, reason, affected paths, and host-redacted command. Route responses by thread/turn/item/approval ID and never through the normal prompt queue.

Decision rules:

- Multiple pending decisions remain separate and are ordered by arrival within each task. Show one sheet at a time; after it resolves or expires, show the next. Home shows each affected task as waiting, not a separate attention screen.
- A response must match the exact pending thread/turn/item/approval ID and decision type. One sheet can never answer another request; stale/cross-request responses are rejected.
- `Not now` dismisses a question locally without sending an answer. The task stays `Needs your answer` and the question can be reopened. Approval sheets have no `Not now`; they expose only Codex-offered allow scopes and Deny.
- If Codex marks a question `isSecret`, the phone does not accept or transmit the value. It shows `Answer on computer` because passwords/API keys must remain on the computer.
- Host redaction replaces each removed span with an explicit typed marker such as `<redacted:secret>` while preserving executable token/order structure. If safe redaction prevents the complete command from being understood, phone approval is disabled and the user is told to review it on the computer.

**Verify:** `go test ./companion/internal/decisions ./companion/internal/mobileapi/transport ./companion/integration/decisions -race`; Android UI test for every branch; live destructive commands are not used for verification.

### Task 12: Implement app drawer, Appearance, and foreground connection service

**Objective:** Complete approved screens 4, 6, and 7 and deliver background task-state notifications without Firebase.

**Files:**

- Create: `android/app/src/main/kotlin/app/codexlauncher/launcher/apps/AppDrawerScreen.kt`
- Create: `android/app/src/main/kotlin/app/codexlauncher/launcher/apps/InstalledAppsRepository.kt`
- Create: `android/app/src/main/kotlin/app/codexlauncher/connection/stream/CodexConnectionService.kt`
- Create: `android/app/src/main/kotlin/app/codexlauncher/connection/stream/StreamClient.kt`
- Modify: `android/app/src/main/AndroidManifest.xml`

**Red tests:** App search/launch, Android Settings access, hidden/unlaunchable packages, foreground notification creation, notification denial/channel removal, default-launcher cold start, reboot/unlock, start rejection, process kill, force-stop, Tailscale restart/update, reconnect, service stop, VPN loss, turn reply, approval, question, and failure notifications.

**Implementation:** Proceed only after Task 2 proves the Android 16 lifecycle. Use Android launcher APIs rather than broad package scraping. Declare the proven foreground-service type and target-SDK permissions. Notifications say “Codex replied” for `turn/completed`, never “Task completed.” Appearance contains only the theme control, previews, and Android-settings note.

**Verify:** Instrumentation on Android 16 with screen off, Doze, Wi-Fi→5G, service stop/restart, and Tailscale disconnect.

### Task 13: Install, replace, roll back, and verify the companion on all desktop platforms

**Objective:** Start the companion after the user's next login/reboot, replace it safely, roll back locally, and uninstall without leaving credentials or services behind.

**Files:**

- Create: `companion/internal/hostinstall/launchd.go`
- Create: `companion/internal/hostinstall/systemd.go`
- Create: `companion/internal/hostinstall/windows.go`
- Create: `companion/internal/hostinstall/install_test.go`
- Modify: `companion/internal/cli/root.go`
- Modify: `companion/internal/cli/commands.go`
- Modify: `companion/internal/app/app.go`
- Create: `release/checks/companion-smoke.sh`
- Create: `release/checks/companion-smoke.ps1`

**Red tests:** Install, start after next login, status, atomic local replacement, rollback, uninstall, missing Codex, broken Codex, missing Tailscale, port collision, identity-key loss, and user-config migration.

**Implementation:** One Go CLI provides `setup`, `pair`, `devices`, `revoke`, `status`, `doctor`, `install`, `install --replace <verified-local-artifact>`, `rollback`, `uninstall`, and `version`. Replacement stops the user service, verifies checksum/provenance, preserves the prior binary for one rollback, migrates config transactionally, and restarts. There is no network self-updater in V1. Use a per-user LaunchAgent on macOS, systemd user service on Linux, and current-user Task Scheduler entry on Windows. Promise startup after login, not operation while logged out. Do not require administrator access unless an OS forces it, and report that before changing the system.

**Verify:** On each OS, run the real Codex CLI/app-server through initialize, start/read a test thread, receive an event, route a safe approval denial, restart/reconcile, confirm credentials remain local, and exercise install/login-start/replace/rollback/uninstall. Use native macOS; Linux ARM VM through QEMU plus GitHub Ubuntu x64; Windows 11 ARM VM through QEMU where licensed media is available plus GitHub Windows x64. Any real model-backed smoke requires the account/cost approval gate. A platform remains `experimental` until its native real-Codex smoke passes.

### Task 14: CI, reproducible releases, documentation, and first public alpha

**Objective:** Produce installable, checksummed artifacts that another technical user can set up without private help.

**Files:**

- Create: `.github/workflows/android.yml`
- Create: `.github/workflows/companion.yml`
- Create: `.github/workflows/release.yml`
- Create: `docs/setup/android.md`
- Create: `docs/setup/companion.md`
- Create: `docs/security/threat-model.md`
- Create: `docs/compatibility/codex.md`

**Red release check:** A clean machine cannot pass until it can verify checksums, install Tailscale/Codex separately, install the companion, pair, select a project, start a task, receive a reply, deny an approval, stop a turn, go offline, recover, and uninstall.

**Implementation:**

1. Build signed Android APK/AAB and Go archives for darwin/linux/windows on amd64/arm64.
2. Generate SHA-256 checksums, SBOMs, dependency/license notices, and provenance.
3. Keep Android signing keys and any future CI Tailscale credentials outside the repository.
4. Publish through GitHub Releases only after explicit approval to create/push the repository and use the chosen GitHub account.
5. Document that users bring their own Tailscale and Codex accounts; no credentials are sent to the project maintainers.
6. Treat desktop archives as unsigned technical-alpha artifacts until Apple notarization and Windows code-signing accounts/costs are explicitly approved. Document Gatekeeper/SmartScreen warnings truthfully; do not call unsigned artifacts consumer-ready.

**Verify:** CI from a clean clone, dependency/security scans, Android lint, Go vet/test/race, schema compatibility fixtures, and the complete manual release checklist on Pixel 9 + macOS plus platform VM results.

### Task 15: Run a fresh final implementation judge and close every P1/P2 gap

**Objective:** Verify the finished product against an independently defined quality bar rather than the assumptions of the implementation context.

**Inputs:** Final source tree, protocol fixtures, threat model, red/green test logs, Pixel 9 recordings/screenshots, Codex compatibility report, macOS/Windows/Linux smoke reports, release archives, checksums, and install/uninstall evidence.

**Judge procedure:** A fresh agent first defines what good looks like for product fidelity, protocol truth, crash safety, Android 16 lifecycle, security/privacy, cross-platform support, visual matching, accessibility, diagnostics, distribution, and user setup. Only then may it inspect the implementation and evidence. It must quote exact file/line and test evidence for every finding.

**Gate:** Any P1 or P2 returns work to the relevant task, adds/revises a failing test first, and triggers a full rerun of affected integration/E2E suites. `READY` requires zero open P1/P2 findings and a second clean judge pass after corrections.

## Test coverage map

```text
PHONE FLOW                                  COMPANION / CODEX PATH
First launch                               setup → Tailscale/Codex checks
  ├── no Tailscale → setup help              ├── missing/broken binary → clear error
  ├── pair success → signing key in Keystore  ├── code expiry/reuse → deny
  └── pair failure → recoverable              └── bind outside tailnet → refuse

Home prompt [E2E]
  ├── no project → disabled                 action state → reconcile/unknown
  ├── idle task → turn/start                  ├── idle → submit
  ├── busy + queue → durable order            ├── busy → store and later submit
  ├── busy + redirect → capability check      └── crash → replay or outcome unknown
  └── offline → Computer offline; draft retained encrypted

Live transcript [E2E]
  ├── delta/item/diff → mapped UI            app-server event → adapter → journal
  ├── unknown item → generic safe row          ├── warm cursor → replay
  ├── turn/completed → “Codex replied”         └── compacted cursor → snapshot
  └── failed/interrupted → recovery text

Decision path [E2E, security-critical]
  ├── approval allow/deny                    exact request ID → app-server response
  ├── timeout/disconnect → fail closed       expired/replayed ID → reject
  └── question capability/fallback           experimental absent → plain message
```

Unit tests cover every pure mapper, state transition, validation rule, queue branch, and redaction rule. Integration tests cover WebSocket reconnect, SQLite restart, fake app-server JSON-RPC, pairing/TLS, and installer lifecycle. End-to-end tests cover the user journeys above. Live Codex tests are a small separately approved suite because they use the user's authenticated account.

## Diagnostics required from the first implementation

- Android logs use the Task 1 structured logger wrapper with tags `[pairing]`, `[connection]`, `[sync]`, `[task-control]`, and `[decision]`.
- Companion logs use `slog` with the same feature tags and request IDs.
- Log input shape/count, chosen branch and reason, output shape/count, and full contextual errors.
- Redact prompts, assistant text, commands, file paths where unnecessary, file contents, tokens, certificates, QR secrets, ChatGPT state, and environment values.
- `doctor` reports versions, reachability, service state, tailnet address, pinned identity fingerprint, schema compatibility, and last non-secret error.

## Failure modes and visible recovery

| Failure | Test | Handling | User sees |
|---|---|---|---|
| Computer sleeps or Tailscale disconnects | Connection E2E | Close stream, preserve encrypted draft, clear work content and cursor, backoff; next connection requests full snapshot | `Computer offline` and last successful connection time |
| WebSocket frame repeats | Protocol integration | Return stored action result | No duplicate prompt or approval |
| Companion crashes after Codex send | Crash-injection integration | Reconcile proven state or block blind retry | `Outcome unknown` with inspect/retry choice |
| Companion restarts mid-turn | Restart integration | Rejoin/read thread, replay journal or snapshot | `Reconnecting`, then current known state |
| Codex schema changes | Compatibility fixtures | Capability/version gate | Upgrade companion/Codex instruction, no unsafe guess |
| Approval expires during disconnect | Security E2E | Reject late response | `Approval expired`; no access granted |
| Project folder disappears | Project unit/UI | Clear selection and block send | Choose another project |
| Project path escapes by symlink/junction | Cross-platform path security | Canonicalize and reject outside approved root | Folder unavailable; no path contents disclosed |
| Attachment is corrupt or interrupted | Binary-contract integration | Reject hash/order mismatch and delete temporary file | Upload failed with retry/cancel |
| Upload quota or disk capacity is exhausted | Resource-limit integration | Reject before allocation or abort safely and clean partial data | Storage limit reached; retry after cleanup |
| Android kills connection service | Android instrumentation | Clear work content and reconnect when launcher resumes | `Computer offline` until a fresh sync completes |
| Tailnet device reaches port without pairing | Pairing security | Host pin plus enrolled device signature required | No information disclosed beyond generic denial |
| Companion identity changes | Pairing security | Refuse new identity and require explicit re-pair | Computer identity changed |
| Event cursor is too old | Journal integration | Send bounded fresh snapshot | Short resync state, no duplicate visible entries |
| Global Codex path is broken | Installer smoke | Use configured valid path or stop | Exact failing path and repair instruction |

## Parallel implementation lanes

| Lane | Work | Depends on |
|---|---|---|
| A | Protocol schemas and fixtures | Task 2 findings |
| B | Companion adapter, pairing, journal, queue | A |
| C | Android launcher shell and design system | Task 0 visual update |
| D | Android connection, task UI, decisions | A + B + C |
| E | Desktop installers and cross-platform CI | Companion CLI skeleton |
| F | Release/docs/security | B + D + E |

After Task 2 clears the Codex and Android feasibility gates, lanes B and C can proceed in parallel worktrees. Lane E can begin after the companion command surface stabilizes. Lane D waits for both protocol and Android shell. Lane F is last. Protocol files are owned by lane A to avoid contract merge conflicts.

## NOT in scope for V1

- Hosted relay, Firebase, UnifiedPush, billing, telemetry, accounts, or a project-operated backend.
- Play Store publication. V1 distributes through GitHub Releases; Play review and `QUERY_ALL_PACKAGES` policy analysis happen only if Play distribution is chosen.
- Multiple computers on one phone, cross-device handoff, or sharing control of one computer with another person.
- Windows/Linux graphical tray apps. The companion is a CLI plus background user service.
- Full Hermes parity for goals, scheduled tasks, memory, profiles, rollback/undo semantics, subagent trees, interactive terminals, or plugin management.
- Live voice conversation, video, location, or a second notification center.
- Calling ChatGPT's cloud Remote protocol, copying ChatGPT auth to Android, or
  binding raw desktop IPC/app-server to Tailscale. The approved same-user local
  desktop follower adapter is explicitly in scope.
- A semantic “task completed” screen. V1 reports turn replies and observable task state only.

## Implementation blockers to clear immediately before autonomous work

1. For the first valid live desktop write/model call, state the exact
   authenticated ChatGPT account/workspace, expected handful of calls or credit
   use, and obtain explicit approval.
2. If Windows VM media or a paid VM product is needed, identify its
   license/account/cost and obtain approval before download or use.
3. Before creating a remote repository or release, identify the GitHub
   account/organization and obtain approval to create, push, and publish.

## Definition of done

- Pixel 9 can become the default launcher and always escape to All apps and Android Settings.
- A new user can install official Tailscale and Codex, install the companion, pair one computer, select/change an approved project folder, and see both computer and project before sending.
- On macOS, the phone lists, reads, and safely controls the same desktop-owned
  Codex tasks through the pinned follower adapter. An incompatible desktop
  update fails closed instead of creating a separate task.
- Text, tool activity, diffs, rename/archive/fork/resume, model/reasoning/permission choices, queue/redirect/stop, dictation, attachments, approvals, questions/fallback, offline state, and reply notifications work through disconnect/restart tests.
- Offline always shows `Computer offline`; task contents are memory-only and absent after disconnect/process death, while the unfinished draft remains encrypted for recovery.
- Pixel 9 pairing uses a non-exportable hardware-backed device signing key; reduced-protection devices warn or fail exactly as Task 5 specifies.
- Notifications remain generic and retain no task name or work content in Android notification history.
- Every cold start or content clear uses `no_local_state` and rebuilds from a full snapshot/thread read.
- Attachment concurrency/temp-byte quotas and disk-full paths reject safely and remove partial files.
- No ChatGPT/Codex or third-party service credential leaves the computer; the phone holds only its own pairing/signing material. No raw app-server listens on the tailnet, and unpaired devices learn nothing useful.
- macOS desktop-owned control passes native live tests. Windows follower control
  remains experimental until its named-pipe path passes the same VM tests.
  Linux supports public app-server/CLI-owned tasks and clearly states that no
  ChatGPT Desktop host exists there.
- Android unit/UI/instrumentation, Go unit/integration/race, protocol fixtures, security cases, and release smoke tests are green in the same run.
- Another technical user can install from the README without private instructions.
- Task 15's fresh implementation judge returns `READY` with no open P1/P2 after any required correction pass.

## Independent verification

A fresh judge must grade this plan from first principles against product fidelity, protocol truthfulness, Android 16 behavior, security, cross-platform distribution, TDD coverage, failure recovery, and the approved visual system. Any gap rated P1 or P2 is folded into this same file before implementation begins.
