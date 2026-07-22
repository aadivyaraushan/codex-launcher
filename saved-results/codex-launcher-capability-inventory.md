# Codex Launcher complete capability inventory

Date: 2026-07-19

## Purpose and scope

This is the audit checklist for every capability implemented by the current Codex Launcher repository. It separates:

- **Pixel**: behavior a person can exercise in the Android app.
- **System**: a supporting security, storage, protocol, or recovery guarantee that needs device inspection or automated fault injection rather than a tap alone.
- **Host**: companion CLI/service behavior that cannot be driven through the Pixel UI.

An error branch is listed separately when it produces a distinct user-visible recovery path or protects work. Pure internal implementation details are not mislabeled as product capabilities. Features explicitly excluded from V1 are recorded at the end.

Status legend:

- `SOURCE`: confirmed in current production code, but not yet rerun in this audit.
- `PASS`: verified against the current installed build and environment during this audit.
- `FAIL`: reproduced failure requiring a fix.
- `BLOCKED`: requires the user-owned approval or external state named in the row.

## Source map

| Code | Current source of truth |
|---|---|
| LA | `android/app/src/main/kotlin/app/codexlauncher/LauncherActivity.kt` |
| SC | `android/app/src/debug/kotlin/app/codexlauncher/debug/scenarios/ScenarioCatalog.kt` |
| PAIR | `connection/pairing/PairingScreen.kt`, `PairingViewModel.kt`, and `network/` |
| CONN | `connection/runtime/LauncherSessionViewModel.kt`, `connection/stream/`, and `connection/session/` |
| HOME | `launcher/home/HomeScreen.kt` and `HomeUiState.kt` |
| PROJECT | `project/selection/ProjectSelector.kt` and `ProjectSelectionViewModel.kt` |
| TASK | `task/transcript/TaskScreen.kt` and `TaskTranscriptState.kt` |
| CONTROL | `task/control/TaskControls.kt` and `TaskControlViewModel.kt` |
| MANAGE | `task/management/TaskActionsMenu.kt` and `TaskActionBridge.kt` |
| DECISION | `decision/approval/ApprovalSheet.kt`, `decision/question/QuestionSheet.kt`, and companion decision router |
| ATTACH | `task/attachments/` and `companion/internal/attachments/` |
| APPS | `launcher/apps/` and `appearance/settings/AppearanceScreen.kt` |
| STORAGE | `storage/`, Android manifest, and `docs/security/threat-model.md` |
| CLI | `companion/internal/cli/root.go`, `commands.go`, and companion host lifecycle packages |
| PLAN | `planning/codex-launcher-v1-plan.md`, especially Tasks 9–12 and Definition of done |

## 1. Launcher shell and navigation

| ID | Surface | Capability | Evidence source | Required verification | Status |
|---|---|---|---|---|---|
| L01 | Pixel | Register as an Android Home app and receive `MAIN + HOME` intents | Android manifest; LA `handleIncomingIntent` | Select as Home app, press Home from another app, confirm launcher Home | SOURCE |
| L02 | Pixel | Choose Pairing or Home as the root according to stored pairing | LA `PairingRecordState.startDestination` | Cold start paired and unpaired | SOURCE |
| L03 | Pixel | System Back returns through detail → task → Home and Appearance → Apps | LA `BackHandler` | Drive every destination and hardware Back | SOURCE |
| L04 | Pixel | All Apps remains reachable from Pairing, Home, Project, and recovery | PAIR, HOME, PROJECT, `LauncherDialogs.kt` | Tap from each state | SOURCE |
| L05 | Pixel | Android Settings remains reachable from Pairing, Home, Project, Apps, and recovery | Same as L04; LA `openAndroidSettings` | Tap from each state and confirm Settings foreground | SOURCE |
| L06 | Pixel | Loading screen while pairing/storage state is unresolved | LA; `LauncherLoadingScreen` | Cold-start under delayed storage read | SOURCE |
| L07 | Pixel | Fail-closed local-storage recovery screen with Retry and Remove local data | LA; `LocalStateRecoveryScreen`; SC `RECOVERY` | Inject storage failure, test both recovery actions | SOURCE |
| L08 | Pixel | Text size, contrast, and motion inherit Android accessibility settings | theme and setup docs | Large text, contrast, reduced-motion inspection | SOURCE |

## 2. Pairing and computer ownership

| ID | Surface | Capability | Evidence source | Required verification | Status |
|---|---|---|---|---|---|
| P01 | Pixel | Enter a one-time pairing link manually | PAIR; SC `PAIRING_MANUAL` | Fresh manual pair through Fly relay | PASS: paired Pixel 9 through the Fly relay at 12:51 UTC |
| P02 | Pixel | Scan a pairing QR using the camera | PAIR `CameraQrScanner` | Display trusted QR and scan | SOURCE |
| P03 | Pixel | Request camera permission only when QR scanning is chosen | PAIR; SC `PAIRING_QR_PERMISSION` | Revoke permission, choose Scan QR, grant/deny | SOURCE |
| P04 | Pixel | Retry QR scanning after a failed/expired scan | PAIR `Scan again` | Feed invalid QR then retry valid QR | SOURCE |
| P05 | Pixel | Reject malformed, unsafe, private-address, or incompatible pairing links | `PairingOffer`, `PairingValidation`, `SafePublicDns`; SC `PAIRING_MANUAL_ERROR` | Run invalid-link matrix on phone plus tests | SOURCE |
| P06 | Pixel | Show pairing progress and safe errors without leaking the link | PAIR `Pairing…`; diagnostics | Pair and inspect screen/logs | PASS: Pixel showed `Pairing…`; companion recorded the new device without logging the link |
| P07 | Pixel | Retry saving when remote pairing succeeds but local storage fails | PAIR; SC `PAIRING_SAVE_RETRY` | Fault-inject local save failure, retry | SOURCE |
| P08 | System | Pin separate host proof and TLS identities | `HostIdentityPin`, `PinnedTlsClientFactory`, pairing transport | Inspect stored record; reject mismatched cert/proof | SOURCE |
| P09 | System | Generate a hardware-backed phone signing key when Pixel hardware supports it | `PairingKeyStore`, `AndroidDevicePairingSigner` | Inspect KeyInfo after pairing | SOURCE |
| P10 | Pixel | Pair exactly one phone to one fixed computer | pairing service and V1 policy | Confirm second pairing is blocked until revoke/remove | PASS: companion refused a new code while the prior Pixel record existed |
| P11 | Pixel | Open Manage computer from Home | HOME header; LA `unpairConfirmVisible` | Tap computer header | SOURCE |
| P12 | Pixel | Explain and cancel computer removal | `UnpairConfirmationDialog` | Open dialog and cancel without data loss | SOURCE |
| P13 | Pixel/System | Remove pairing, project, draft, action records, and keys atomically while retaining computer tasks | LA `removeLocalData`; `LocalStateWiper` | Create disposable state, remove, inspect data, re-pair | BLOCKED: destructive live approval |
| P14 | Host | Revoke a specific phone and close its live sessions | pairing service; CLI `revoke` | Revoke exact test device and observe disconnect | PASS: revoked only the stale Pixel record after the test runner erased its phone key |
| P15 | Host | List paired devices | CLI `devices` | Compare CLI device with Pixel pairing | PASS: CLI listed one `Pixel 9` matching the connected phone |

## 3. Connection, synchronization, and background operation

| ID | Surface | Capability | Evidence source | Required verification | Status |
|---|---|---|---|---|---|
| C01 | System | Phone and companion connect outbound through Fly while relay remains content-blind | relay client/server and threat model | Live paired session plus relay ciphertext evidence | SOURCE |
| C02 | System | Authenticate each session with fresh nonce, device signature, and pinned host identity | `SessionHandshake`, companion pairing/session code | Replay/mismatch fault tests and live handshake | SOURCE |
| C03 | Pixel | Show Connecting state | HOME policy; SC `HOME_CONNECTING` | Cold reconnect capture | SOURCE |
| C04 | Pixel | Show Syncing state before content becomes usable | HOME policy; SC `HOME_SYNCING` | Delay snapshot and capture | SOURCE |
| C05 | Pixel | Show Online state with current computer, projects, and tasks | HOME; CONN | Live connection capture | PASS: current build showed MacBook Pro, approved project, and live task rows |
| C06 | Pixel | Distinguish relay unreachable from computer offline | README; connection state machine | Cut relay, then cut companion, compare messages | SOURCE |
| C07 | Pixel | Show incompatible desktop integration and fail closed | SC `HOME_INCOMPATIBLE`; compatibility probe | Inject incompatible probe result | SOURCE |
| C08 | Pixel | Show pairing revoked and stop trusting the session | SC `HOME_REVOKED`; connection policy | Host revoke while connected | BLOCKED: destructive live approval |
| C09 | Pixel | Retry connection manually | HOME `Try again`; LA `connect(force=true)` | Disconnect companion, tap Try again | SOURCE |
| C10 | Pixel | Expand connection help without exposing work content | HOME `Connection help` | Open/close in offline state | SOURCE |
| C11 | System | Automatically reconnect after network or companion loss | connection service/state machine | Toggle network/companion and time recovery | SOURCE |
| C12 | System | Detect sequence gaps, acknowledge snapshots/events, and resync safely | `SequenceAcknowledgementGate`, session client | Fault-injected gap/replay plus live reconnect | SOURCE |
| C13 | Pixel | Show last successful connection label while offline | `LastConnectedLabel`, last-seen store | Connect, disconnect, verify label | SOURCE |
| C14 | System | Keep the sealed connection in a foreground Android service | `CodexConnectionService` | Background app, inspect service/process | SOURCE |
| C15 | Pixel | Request notification permission and warn if background service cannot run | LA; `BackgroundConnectionWarningDialog` | Revoke notification permission and connect | PASS (permission request): Android permission dialog appeared immediately after fresh pairing; denial warning still pending |

## 4. Approved project selection

| ID | Surface | Capability | Evidence source | Required verification | Status |
|---|---|---|---|---|---|
| J01 | Pixel | List only companion-approved opaque project choices | PROJECT; companion projects package | Compare phone choices with companion config | PASS: companion reported one approved project and Pixel listed only `Home folder` |
| J02 | Pixel | Require a project before starting a new task | HOME policy and composer send gate | Clear selection and verify Send disabled | SOURCE |
| J03 | Pixel | Select a project | PROJECT `onSelect` | Select disposable approved project | PASS: selected `Home folder`; Home changed from `Choose project` to `Home folder` |
| J04 | Pixel | Change the selected project from Home | HOME `onChooseProject` | Switch projects and return | SOURCE |
| J05 | Pixel | Show No approved projects safely | SC `PROJECT_EMPTY` | Start companion with zero project choices | SOURCE |
| J06 | Pixel | Retry storing a remotely accepted selection | SC `PROJECT_SAVE_RETRY` | Fault-inject storage write and retry | SOURCE |
| J07 | System | Persist only opaque project ID/display name, not unrestricted paths | project store and threat model | Inspect app data after selection | SOURCE |

## 5. Home task list and new-task composer

| ID | Surface | Capability | Evidence source | Required verification | Status |
|---|---|---|---|---|---|
| H01 | Pixel | List recent desktop-owned and CLI-owned Codex tasks | HOME; task adapter/catalog | Compare phone rows with Mac sessions | SOURCE |
| H02 | Pixel | Show observable task states: working, waiting, replied/idle, failed, interrupted | task state mapper; PLAN Task 9 | Produce each disposable state | BLOCKED: live task approval |
| H03 | Pixel | Update task rows in real time | CONN event reducer | Send Mac-side updates and measure phone latency | SOURCE |
| H04 | Pixel | Open a task from its Home row | HOME `onOpenTask`; LA `openTask` | Tap several current tasks | SOURCE |
| H05 | Pixel | Focus and edit “What do you want done?” with the input remaining visible above the keyboard | HOME `OutlinedTextField` and focus scroll | Tap, type, rotate/focus, capture layout | PASS: Pixel IME open; Prompt `[53,461][1027,655]`, action row `y=655–781`; screenshot `.context/final-prompt-focused.png` |
| H06 | Pixel/System | Encrypt and persist an unfinished new-task draft across process death | `DraftComposerViewModel`, `EncryptedDraftStore` | Type without send, force-stop, reopen, inspect ciphertext | SOURCE |
| H07 | Pixel | Preserve editable text and show a safe message when draft save fails | SC `HOME_DRAFT_ERROR` | Fault-inject save failure | SOURCE |
| H08 | Pixel | Choose among host-advertised models | `NewTaskOptionControls`, companion task options | Open menu and select each advertised model | SOURCE |
| H09 | Pixel | Choose model-supported reasoning level | same as H08 | Select each advertised reasoning level | SOURCE |
| H10 | Pixel | Choose host-advertised permission mode and show its description | same as H08 | Select workspace/read-only/full-access options actually offered | SOURCE |
| H11 | Pixel | Reset unsafe full-access selection on a new authenticated session | HOME test and options session key | Select full access, reconnect/new session, confirm reset | SOURCE |
| H12 | Pixel | Start a new task using selected project/model/reasoning/permission | LA `startNewTask`; companion task adapter | Create disposable task and compare Mac | BLOCKED: live task approval and account confirmation |
| H13 | Pixel | Disable duplicate sends while a task start is unresolved | composer state/action journal | Double-tap under delayed response | SOURCE |
| H14 | Pixel | Show “Outcome unknown” review gate and require explicit computer check | SC `HOME_NEW_TASK_REVIEW` | Drop response after send and exercise gate | BLOCKED: live task approval |
| H15 | Pixel | Retain the draft and explain a definitive task-start failure | SC `HOME_NEW_TASK_ERROR` | Reject start and inspect retained text | BLOCKED: live task approval |
| H16 | Pixel | Add dictation text to the editable draft without sending phone audio to the companion | `PromptDictationContract`; LA | Speak known phrase, inspect editable text/network | BLOCKED: phone unlocked and audible speech approval |

## 6. Attachments

| ID | Surface | Capability | Evidence source | Required verification | Status |
|---|---|---|---|---|---|
| A01 | Pixel | Open a privacy explanation before choosing an attachment | `AttachmentChoiceDialog` | Tap Attach and dismiss | SOURCE |
| A02 | Pixel | Pick an image using Android’s photo picker | LA `PickVisualMedia` | Push disposable image, select it | SOURCE |
| A03 | Pixel | Pick a file using Android’s document picker | LA `OpenDocument` | Push disposable text/PDF, select it | SOURCE |
| A04 | Pixel | Display selected attachment names and remove before sending | `AttachmentRows` | Add two disposable files, remove each | SOURCE |
| A05 | Pixel | Enforce two-file, size, invalid-file, and supported-type limits with clear errors | `AttachmentDocumentReader`, uploader | Exercise each rejection | SOURCE |
| A06 | Pixel | Reject audio and video; allow images, text, PDF, JSON, and ZIP | LA MIME list and attachment reader | Picker/reader matrix | SOURCE |
| A07 | System | Upload authenticated ordered chunks with declared size and SHA-256 verification | Android uploader; companion attachment store | Live upload plus tamper fault test | SOURCE |
| A08 | System | Resume interrupted upload from companion’s durable offset | attachment protocol/store | Interrupt network mid-upload and resume | SOURCE |
| A09 | System | Cancel and securely delete removed/failed/orphaned/expired uploads | attachment store | Remove, fail, restart, expire and inspect disk | SOURCE |
| A10 | System | Enforce per-file, per-device, global, temporary-byte, and expiry quotas | attachment quota/store | Automated fault matrix | SOURCE |
| A11 | Pixel | Send attachments with new tasks, follow-ups, and redirects | LA; task adapter | Disposable task flows with photo and file | BLOCKED: live task approval |

## 7. Task transcript and details

| ID | Surface | Capability | Evidence source | Required verification | Status |
|---|---|---|---|---|---|
| T01 | Pixel | Load the selected task transcript | TASK; CONN | Open several real tasks | SOURCE |
| T02 | Pixel | Render user and assistant messages | transcript mapper/rows | Compare with Mac task | SOURCE |
| T03 | Pixel | Render intermediate reasoning/thinking entries as they arrive | transcript mapper and prior requirement | Run thinking task and compare live | SOURCE |
| T04 | Pixel | Render tool activity and status without dumping unsafe raw payloads | TASK `TranscriptEntryRow` | Run command/file tool task | SOURCE |
| T05 | Pixel | Open full command-output detail | TASK `TranscriptDetail.Command`; SC `TASK_COMMAND_DETAIL` | Tap View output and compare | SOURCE |
| T06 | Pixel | Open per-file change/diff detail | TASK `TranscriptDetail.File`; SC `TASK_FILE_DETAIL` | Tap changed file and compare | SOURCE |
| T07 | Pixel | Load earlier transcript pages using a stable cursor | TASK `Load earlier` | Use a long disposable task and page backward | BLOCKED: live task approval |
| T08 | Pixel | Report shortened long content | TASK `state.truncated` | Generate oversized disposable entry | BLOCKED: live task approval |
| T09 | Pixel | Append live transcript updates without reopening | CONN reducer and task screen | Send Mac-side messages while open | SOURCE |
| T10 | Pixel | Show loading and unavailable states safely | SC `TASK_LOADING`, `TASK_UNAVAILABLE` | Delay and invalidate task | SOURCE |
| T11 | Pixel | Collapse long task titles to two lines and expand/collapse on tap | TASK header | Open a long-title disposable task and tap title | BLOCKED: live task approval |
| T12 | Pixel | Back to Home without persisting transcript content | TASK `onBack`; storage policy | Open, back, force-stop, inspect data | SOURCE |

## 8. Existing-task controls and management

| ID | Surface | Capability | Evidence source | Required verification | Status |
|---|---|---|---|---|---|
| E01 | Pixel | Send a follow-up when the task is idle | CONTROL | Disposable idle task and Mac comparison | BLOCKED: live task approval |
| E02 | Pixel | Queue a follow-up while a turn is active | CONTROL Queue | Long-running disposable task | BLOCKED: live task approval |
| E03 | Pixel | Redirect/steer the current turn when the host advertises support | CONTROL Redirect | Redirect long-running disposable task | BLOCKED: live task approval |
| E04 | Pixel | Fall back safely when redirect is unsupported or state changes | task adapter capability checks | Capability-change fault test | SOURCE |
| E05 | Pixel | Confirm Stop, cancel with Keep working, or stop the current turn | CONTROL stop dialog | Disposable long-running task | BLOCKED: live task approval |
| E06 | Pixel/System | Persist encrypted follow-up drafts separately per task | CONN follow-up draft store | Type in two tasks, force-stop, reopen | SOURCE |
| E07 | Pixel | Keep queued state visible after acceptance | CONTROL `TaskQueueState.QUEUED` | Queue and observe | BLOCKED: live task approval |
| E08 | Pixel | Gate another action when queue/redirect outcome is unknown | CONTROL `OUTCOME_UNKNOWN` | Drop response and exercise “I checked Codex” | BLOCKED: live task approval |
| E09 | Pixel | Attach and dictate into follow-up/redirect composers | CONTROL | Disposable attachment/dictation flow | BLOCKED: live task and speech approval |
| E10 | Pixel | Rename a task | MANAGE | Rename disposable task and compare Mac/Home | BLOCKED: destructive live approval |
| E11 | Pixel | Cancel rename without changing the task | MANAGE rename dialog | Open/cancel disposable task | BLOCKED: live task approval |
| E12 | Pixel | Archive a task after confirmation | MANAGE | Archive disposable task, verify it leaves recent list | BLOCKED: destructive live approval |
| E13 | Pixel | Fork a task and open/identify the resulting task | MANAGE | Fork disposable task and compare Mac | BLOCKED: destructive live approval |
| E14 | Pixel | Block another fork when prior outcome is unknown until computer review | MANAGE unresolved fork gate | Drop fork response and clear review | BLOCKED: destructive live approval |
| E15 | Pixel | Explain unconfirmed rename/archive/fork without retrying blindly | MANAGE failure dialog | Drop response for each action | BLOCKED: destructive live approval |

## 9. Approvals and questions

| ID | Surface | Capability | Evidence source | Required verification | Status |
|---|---|---|---|---|---|
| D01 | Pixel | Show an approval tied to exact computer, project, request, and safe context | DECISION | Trigger harmless disposable approval | BLOCKED: live task/account approval |
| D02 | Pixel | Allow once when Codex offers it | `ApprovalSheet` | Harmless approval request | BLOCKED: live task/account approval |
| D03 | Pixel | Allow for session only when Codex offers it | `ApprovalSheet` | Harmless scoped request | BLOCKED: live task/account approval |
| D04 | Pixel | Deny an approval | `ApprovalSheet` | Harmless request, deny | BLOCKED: live task/account approval |
| D05 | Pixel | Deny and stop when offered | `ApprovalSheet` | Harmless request, deny+stop | BLOCKED: live task/account approval |
| D06 | Pixel | Disable approval when redaction makes a command unclear | SC `APPROVAL_REDACTED`; decision router | Synthetic/fault request | SOURCE |
| D07 | System | Keep multiple decisions separate and ordered; reject stale, duplicate, or cross-request replies | decision router/action journal | Automated FIFO/replay matrix | SOURCE |
| D08 | Pixel | Answer single- and multi-choice questions | `QuestionSheet`; SC `QUESTION_CHOICE` | Disposable task using question tool | BLOCKED: live task/account approval |
| D09 | Pixel | Enter and submit free-text answers | SC `QUESTION_FREE_TEXT` | Disposable question | BLOCKED: live task/account approval |
| D10 | Pixel | Use Not now without sending an answer | `QuestionSheet` | Dismiss and confirm request remains on Mac | BLOCKED: live task/account approval |
| D11 | Pixel | Refuse secret answers on phone and direct the user to the computer | SC `QUESTION_SECRET` | Synthetic/live secret-marked question | SOURCE |
| D12 | Pixel | Show sending state and prevent double responses | SC approval/question sending | Delay response and double tap | SOURCE |
| D13 | System | Protect approval/question screens from screenshots | LA `FLAG_SECURE` | Attempt ADB/recents screenshot during decision | SOURCE |
| D14 | Pixel/System | Use plain-message or deny/cancel fallback when native question/MCP capability is absent | decision fallback and companion router | Capability-disabled integration test | SOURCE |

## 10. App drawer and appearance

| ID | Surface | Capability | Evidence source | Required verification | Status |
|---|---|---|---|---|---|
| X01 | Pixel | Enumerate every enabled `MAIN + LAUNCHER` activity except Codex Launcher | APPS and manifest query | Compare Android count with production log/list | PASS: prior installed-build Pixel proof, rerun planned |
| X02 | Pixel | Scroll from first to final app without leaving the drawer | `AppDrawerScreen` | Long-scroll top to bottom | PASS: prior installed-build Pixel proof, rerun planned |
| X03 | Pixel | Search apps case-insensitively | `AppDrawerScreen` query filter | Search first/middle/final app and clear | SOURCE |
| X04 | Pixel | Launch an app and return to the drawer/Home | `InstalledAppsRepository.launch` | Launch disposable apps and final row | PASS: prior BitLife launch proof, rerun planned |
| X05 | Pixel | Show a recoverable error when Android rejects an app launch | SC `APPS_LAUNCH_ERROR` | Synthetic disabled activity | SOURCE |
| X06 | Pixel | Open Android Settings from the drawer | APPS | Tap and return | SOURCE |
| X07 | Pixel | Open Launcher settings/Appearance from the drawer | APPS | Tap and return | SOURCE |
| X08 | Pixel | Follow system theme | `AppearanceScreen`, theme store | Select and toggle Android theme | SOURCE |
| X09 | Pixel | Force Light theme and persist it | Appearance/theme store | Select, force-stop, reopen | SOURCE |
| X10 | Pixel | Force Dark theme and persist it | Appearance/theme store | Select, force-stop, reopen | SOURCE |

## 11. Notifications

| ID | Surface | Capability | Evidence source | Required verification | Status |
|---|---|---|---|---|---|
| N01 | Pixel | Persistent foreground connection notification | connection notification policy | Background app and inspect shade | SOURCE |
| N02 | Pixel | Notify on task replies while launcher is not visible | notification policy | Disposable task reply in background | BLOCKED: live task/account approval |
| N03 | Pixel | Notify on approval requests | notification policy | Background harmless approval | BLOCKED: live task/account approval |
| N04 | Pixel | Notify on question requests | notification policy | Background disposable question | BLOCKED: live task/account approval |
| N05 | Pixel | Notify on relevant task/connection failures without exposing private content | notification policy | Inject failure and inspect redaction | SOURCE |
| N06 | Pixel | Open the correct launcher/task surface from a notification | notification intent routing | Tap each notification class | BLOCKED: live task/account approval |

## 12. Privacy, local storage, and resilience

| ID | Surface | Capability | Evidence source | Required verification | Status |
|---|---|---|---|---|---|
| S01 | System | Persist no task titles, prompts, replies, commands, code, paths, transcripts, or event cursors | STORAGE; PLAN Task 9 | Populate app, inspect app-private files/database | SOURCE |
| S02 | System | Encrypt unfinished new-task and follow-up drafts with non-exportable Android Keystore keys | draft stores/key store | Inspect ciphertext and KeyInfo | SOURCE |
| S03 | System | Persist only bounded metadata-only action records with expiry/caps | action journal/store | Inspect PREPARED/SENT_UNKNOWN/CONFIRMED records | SOURCE |
| S04 | System | Disable Android backup for app-private state | Android manifest/data extraction rules | `bmgr`/manifest inspection | SOURCE |
| S05 | System | Prevent sensitive task snapshots in Recents | activity/window policy | Open task, enter Recents, inspect card | SOURCE |
| S06 | System | Redact secrets, prompts, commands, paths, tokens, and large values from logs | `AppLog` and companion logger | Seed markers, scan logcat/host logs | SOURCE |
| S07 | System | Fail closed when pairing/project/draft/action storage is unavailable | local write gate/recovery UI | Fault-inject storage failures | SOURCE |
| S08 | System | Restore pairing, selected project, theme, and encrypted drafts after process death | stores and LA startup | Force-stop/relaunch matrix | SOURCE |
| S09 | System | Rebuild task list/transcripts from companion after cold start without local content cache | CONN/session | Force-stop offline/online and inspect | SOURCE |
| S10 | System | Keep action outcomes safe across disconnect/restart and require review when uncertain | action journal/bridges | Disconnect during each action phase | SOURCE |
| S11 | System | Clear private attachment bytes after handoff/removal/failure | uploader/store | Heap/disk lifecycle inspection | SOURCE |
| S12 | System | Keep pairing links and private work out of diagnostics and saved artifacts | logger/security docs | Recursive log/artifact marker scan | SOURCE |

## 13. Companion CLI and host-only capabilities

These are part of the Codex Launcher product but cannot truthfully be described as Pixel-driven features.

| ID | Surface | Capability | Evidence source | Required verification | Status |
|---|---|---|---|---|---|
| K01 | Host | Configure computer name, approved projects, Codex adapter, and Fly relay registration | CLI `setup` | Validate current config or isolated temp config | SOURCE |
| K02 | Host | Install per-user background service | CLI `install` | Isolated/native install cycle | SOURCE |
| K03 | Host | Replace with a verified local artifact while preserving rollback | CLI `install --replace` | Signed/checksummed replacement cycle | SOURCE |
| K04 | Host | Roll back one replacement | CLI `rollback` | Isolated lifecycle cycle | SOURCE |
| K05 | Host | Uninstall service, binary, config, pairing, and local state | CLI `uninstall` | Isolated lifecycle cycle; never current live state without approval | BLOCKED: destructive host approval |
| K06 | Host | Create five-minute single-use pairing offer | CLI `pair` | Pair/replay/expiry checks | SOURCE |
| K07 | Host | List paired devices | CLI `devices` | Compare current Pixel | SOURCE |
| K08 | Host | Revoke one or more exact devices | CLI `revoke` | Exact disposable device | BLOCKED: destructive live approval |
| K09 | Host | Report configured/service/device/project status | CLI `status` | Run current binary | SOURCE |
| K10 | Host | Diagnose Codex, relay, service, reachability, schema, identity, and last error | CLI `doctor` | Run and require all named checks | SOURCE |
| K11 | Host | Report companion version/provenance | CLI `version` | Compare installed/build/release metadata | SOURCE |
| K12 | Host | Run after user login without administrator access where OS permits | host install/service packages | LaunchAgent/systemd/Task Scheduler inspection | SOURCE |
| K13 | Host | Follow the same ChatGPT Desktop-owned tasks on macOS/Windows | desktop follower adapter and compatibility docs | Compare live Conductor task IDs/transcripts | SOURCE |
| K14 | Host | Support public Codex app-server/CLI-owned tasks, including Linux | app-server adapter and compatibility docs | Isolated CLI-owned task on supported host | SOURCE |
| K15 | Host | Fail closed when the private desktop adapter version is incompatible | compatibility probe | Version mismatch fixture/live simulation | SOURCE |

## Explicitly not capabilities in V1

The current product does **not** claim: hosted accounts or billing, Firebase/UnifiedPush, multiple computers on one phone, cross-device handoff, shared control, graphical desktop tray apps, goals/scheduled tasks/memory/profiles/subagent trees, interactive terminals, plugin management, live voice conversations, video, location, a second notification center, cloud Remote protocol, Play Store distribution, a network self-updater, or a semantic “task completed” screen. These exclusions come from `planning/codex-launcher-v1-plan.md` under **NOT in scope for V1**.

## Current audit blockers

1. The Pixel returned to its lock screen during the five-minute instrumentation run. Physical UI driving will resume after the user unlocks it; the agent will not enter the device password.
2. The user approved using the ChatGPT account `aadivya@fermi.ai` for approximately 10–20 short disposable audit tasks and controls. Only tasks named `PHONE AUDIT …` may be changed; existing tasks remain read-only.
3. Dictation still needs explicit approval for an audible known phrase or another real microphone input method.

## Audit evidence so far

- Physical Pixel pairing through Fly: manual link accepted, `Pairing…` shown, one matching `Pixel 9` listed by the companion.
- Physical default-Home proof: Android resolved `MAIN + HOME` to `app.codexlauncher/.LauncherActivity`; pressing Home from Settings returned to Codex Launcher.
- Physical composer proof before the fix: IME open, Prompt and action controls reported `[0,0][0,0]`, and the screenshot showed only a black app area above Gboard.
- Physical composer proof after the fix: IME open, Prompt `[53,461][1027,655]`, Attach `[649,655][775,781]`, Send `[901,655][1027,781]`; `.context/final-prompt-focused.png` shows the field and controls directly above Gboard.
- Fixed UI scenarios: 42/42 production-screen scenarios launched on the Pixel with their expected visible states.
- Scenario interactions: 21/21 Pixel tests passed after the composer/status fix.
- Android JVM tests: 273/273 passed, with zero failures, errors, or skips.
- Companion/relay tests: `go test ./...` passed across all tested packages.
- The one-shot 114-test Android instrumentation run became invalid when the Pixel locked during execution; the repeated `No compose hierarchies found` failures began while `mDreamingLockscreen=true`. The focused suites run while unlocked are the valid evidence. A full rerun remains pending after unlock.

## Resolved failures

1. `H05`: Android already resized the activity for Gboard, while Home also applied `imePadding()`. Removing the duplicate inset stopped the full content area collapsing. Focus now scrolls to the composer row instead of the row after it.
2. Online composer actions disappeared after a status message because the action row was a separate lazy-list item and could be removed from the composed viewport. The actions now stay in the composer item, and message changes scroll that item into view. The previously failing interaction is green in the full 21-test scenario class.
