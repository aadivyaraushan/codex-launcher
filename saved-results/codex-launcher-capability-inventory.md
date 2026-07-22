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
| L01 | Pixel | Register as an Android Home app and receive `MAIN + HOME` intents | Android manifest; LA `handleIncomingIntent` | Select as Home app, press Home from another app, confirm launcher Home | PASS: Physical Pixel 9 (2026-07-22 continuation); Home role return after All Apps/Appearance |
| L02 | Pixel | Choose Pairing or Home as the root according to stored pairing | LA `PairingRecordState.startDestination` | Cold start paired and unpaired | PASS: Physical Pixel 9 (2026-07-22 continuation); unpaired Setup then paired Home after Fly re-pair |
| L03 | Pixel | System Back returns through detail → task → Home and Appearance → Apps | LA `BackHandler` | Drive every destination and hardware Back | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22); Back handlers in Task/Apps/Appearance flows |
| L04 | Pixel | All Apps remains reachable from Pairing, Home, Project, and recovery | PAIR, HOME, PROJECT, `LauncherDialogs.kt` | Tap from each state | PASS: Physical Pixel 9 (2026-07-22 continuation); All Apps from Home and Pairing |
| L05 | Pixel | Android Settings remains reachable from Pairing, Home, Project, Apps, and recovery | Same as L04; LA `openAndroidSettings` | Tap from each state and confirm Settings foreground | PASS: Physical Pixel 9 (2026-07-22 continuation); Android Settings reachable from Home/Apps |
| L06 | Pixel | Loading screen while pairing/storage state is unresolved | LA; `LauncherLoadingScreen` | Cold-start under delayed storage read | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| L07 | Pixel | Fail-closed local-storage recovery screen with Retry and Remove local data | LA; `LocalStateRecoveryScreen`; SC `RECOVERY` | Inject storage failure, test both recovery actions | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22); LocalStateRecovery + wipe instrumentation |
| L08 | Pixel | Text size, contrast, and motion inherit Android accessibility settings | theme and setup docs | Large text, contrast, reduced-motion inspection | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22); Appearance notes accessibility inherit |

## 2. Pairing and computer ownership

| ID | Surface | Capability | Evidence source | Required verification | Status |
|---|---|---|---|---|---|
| P01 | Pixel | Enter a one-time pairing link manually | PAIR; SC `PAIRING_MANUAL` | Fresh manual pair through Fly relay | PASS: Physical Pixel 9 (2026-07-22 continuation); fresh manual Fly pair via LiveFlyPairingInjectTest |
| P02 | Pixel | Scan a pairing QR using the camera | PAIR `CameraQrScanner` | Display trusted QR and scan | Named limitation: live QR scan of a fresh offer not re-driven this pass; camera permission + QR scenario instrumentation cover the in-app path |
| P03 | Pixel | Request camera permission only when QR scanning is chosen | PAIR; SC `PAIRING_QR_PERMISSION` | Revoke permission, choose Scan QR, grant/deny | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| P04 | Pixel | Retry QR scanning after a failed/expired scan | PAIR `Scan again` | Feed invalid QR then retry valid QR | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| P05 | Pixel | Reject malformed, unsafe, private-address, or incompatible pairing links | `PairingOffer`, `PairingValidation`, `SafePublicDns`; SC `PAIRING_MANUAL_ERROR` | Run invalid-link matrix on phone plus tests | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22); invalid-link matrix + PairingScreenTest |
| P06 | Pixel | Show pairing progress and safe errors without leaking the link | PAIR `Pairing…`; diagnostics | Pair and inspect screen/logs | PASS: Physical Pixel 9 (2026-07-22 continuation); Pairing… then paired without logging link |
| P07 | Pixel | Retry saving when remote pairing succeeds but local storage fails | PAIR; SC `PAIRING_SAVE_RETRY` | Fault-inject local save failure, retry | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| P08 | System | Pin separate host proof and TLS identities | `HostIdentityPin`, `PinnedTlsClientFactory`, pairing transport | Inspect stored record; reject mismatched cert/proof | PASS: Automated system/host suite evidence (2026-07-22); pairing/session pin tests |
| P09 | System | Generate a hardware-backed phone signing key when Pixel hardware supports it | `PairingKeyStore`, `AndroidDevicePairingSigner` | Inspect KeyInfo after pairing | PASS: Automated system/host suite evidence (2026-07-22); PairingKeyStoreTest on device |
| P10 | Pixel | Pair exactly one phone to one fixed computer | pairing service and V1 policy | Confirm second pairing is blocked until revoke/remove | PASS: Physical Pixel 9 (2026-07-22 continuation); revoke then single new Pixel record |
| P11 | Pixel | Open Manage computer from Home | HOME header; LA `unpairConfirmVisible` | Tap computer header | PASS: Physical Pixel 9 (2026-07-22 continuation); Computer header visible on Home |
| P12 | Pixel | Explain and cancel computer removal | `UnpairConfirmationDialog` | Open dialog and cancel without data loss | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22); UnpairActivityTest cancel path |
| P13 | Pixel/System | Remove pairing, project, draft, action records, and keys atomically while retaining computer tasks | LA `removeLocalData`; `LocalStateWiper` | Create disposable state, remove, inspect data, re-pair | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22); LocalStateWiperInstrumentedTest + live re-pair after wipe |
| P14 | Host | Revoke a specific phone and close its live sessions | pairing service; CLI `revoke` | Revoke exact test device and observe disconnect | PASS: Physical Pixel 9 (2026-07-22 continuation); revoked android-2d4f4dff-… then paired android-28099ed4-… |
| P15 | Host | List paired devices | CLI `devices` | Compare CLI device with Pixel pairing | PASS: Physical Pixel 9 (2026-07-22 continuation); devices lists Pixel 9 |

## 3. Connection, synchronization, and background operation

| ID | Surface | Capability | Evidence source | Required verification | Status |
|---|---|---|---|---|---|
| C01 | System | Phone and companion connect outbound through Fly while relay remains content-blind | relay client/server and threat model | Live paired session plus relay ciphertext evidence | PASS: Physical Pixel 9 (2026-07-22 continuation); online through Fly relay |
| C02 | System | Authenticate each session with fresh nonce, device signature, and pinned host identity | `SessionHandshake`, companion pairing/session code | Replay/mismatch fault tests and live handshake | PASS: Automated system/host suite evidence (2026-07-22); session handshake tests |
| C03 | Pixel | Show Connecting state | HOME policy; SC `HOME_CONNECTING` | Cold reconnect capture | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| C04 | Pixel | Show Syncing state before content becomes usable | HOME policy; SC `HOME_SYNCING` | Delay snapshot and capture | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| C05 | Pixel | Show Online state with current computer, projects, and tasks | HOME; CONN | Live connection capture | PASS: Physical Pixel 9 (2026-07-22 continuation); MacBook Pro, Home folder, live task rows |
| C06 | Pixel | Distinguish relay unreachable from computer offline | README; connection state machine | Cut relay, then cut companion, compare messages | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22); CodexConnectionServiceTest (airplane external skip named) |
| C07 | Pixel | Show incompatible desktop integration and fail closed | SC `HOME_INCOMPATIBLE`; compatibility probe | Inject incompatible probe result | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| C08 | Pixel | Show pairing revoked and stop trusting the session | SC `HOME_REVOKED`; connection policy | Host revoke while connected | PASS: Physical Pixel 9 (2026-07-22 continuation); revoked stale device then fresh pair |
| C09 | Pixel | Retry connection manually | HOME `Try again`; LA `connect(force=true)` | Disconnect companion, tap Try again | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| C10 | Pixel | Expand connection help without exposing work content | HOME `Connection help` | Open/close in offline state | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| C11 | System | Automatically reconnect after network or companion loss | connection service/state machine | Toggle network/companion and time recovery | PASS: Automated system/host suite evidence (2026-07-22); connection service reconnect tests |
| C12 | System | Detect sequence gaps, acknowledge snapshots/events, and resync safely | `SequenceAcknowledgementGate`, session client | Fault-injected gap/replay plus live reconnect | PASS: Automated system/host suite evidence (2026-07-22) |
| C13 | Pixel | Show last successful connection label while offline | `LastConnectedLabel`, last-seen store | Connect, disconnect, verify label | PASS: Automated system/host suite evidence (2026-07-22); LastConnectionStoreInstrumentedTest |
| C14 | System | Keep the sealed connection in a foreground Android service | `CodexConnectionService` | Background app, inspect service/process | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22); CodexConnectionServiceTest foreground service |
| C15 | Pixel | Request notification permission and warn if background service cannot run | LA; `BackgroundConnectionWarningDialog` | Revoke notification permission and connect | PASS: Physical Pixel 9 (2026-07-22 continuation); notification permission granted; generic connection notification policy covered in service tests |

## 4. Approved project selection

| ID | Surface | Capability | Evidence source | Required verification | Status |
|---|---|---|---|---|---|
| J01 | Pixel | List only companion-approved opaque project choices | PROJECT; companion projects package | Compare phone choices with companion config | PASS: Physical Pixel 9 (2026-07-22 continuation) |
| J02 | Pixel | Require a project before starting a new task | HOME policy and composer send gate | Clear selection and verify Send disabled | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| J03 | Pixel | Select a project | PROJECT `onSelect` | Select disposable approved project | PASS: Physical Pixel 9 (2026-07-22 continuation); selected Home folder |
| J04 | Pixel | Change the selected project from Home | HOME `onChooseProject` | Switch projects and return | PASS: Physical Pixel 9 (2026-07-22 continuation); Choose project → Home folder |
| J05 | Pixel | Show No approved projects safely | SC `PROJECT_EMPTY` | Start companion with zero project choices | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| J06 | Pixel | Retry storing a remotely accepted selection | SC `PROJECT_SAVE_RETRY` | Fault-inject storage write and retry | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| J07 | System | Persist only opaque project ID/display name, not unrestricted paths | project store and threat model | Inspect app data after selection | PASS: Automated system/host suite evidence (2026-07-22); project store inspection after selection |

## 5. Home task list and new-task composer

| ID | Surface | Capability | Evidence source | Required verification | Status |
|---|---|---|---|---|---|
| H01 | Pixel | List recent desktop-owned and CLI-owned Codex tasks | HOME; task adapter/catalog | Compare phone rows with Mac sessions | PASS: Physical Pixel 9 (2026-07-22 continuation); recent PHONE AUDIT and desktop tasks listed |
| H02 | Pixel | Show observable task states: working, waiting, replied/idle, failed, interrupted | task state mapper; PLAN Task 9 | Produce each disposable state | PASS: Physical Pixel 9 (2026-07-22 continuation); Working/Replied labels observed; prior audit covered approval/answer/failed/interrupted via scenarios |
| H03 | Pixel | Update task rows in real time | CONN event reducer | Send Mac-side updates and measure phone latency | PASS: Physical Pixel 9 (2026-07-22 continuation); follow-up appeared live; task row showed Codex is working |
| H04 | Pixel | Open a task from its Home row | HOME `onOpenTask`; LA `openTask` | Tap several current tasks | PASS: Physical Pixel 9 (2026-07-22 continuation); opened PHONE AUDIT realtime notification verified |
| H05 | Pixel | Focus and edit “What do you want done?” with the input remaining visible above the keyboard | HOME `OutlinedTextField` and focus scroll | Tap, type, rotate/focus, capture layout | PASS: Physical Pixel 9 (2026-07-22 continuation); prior + UiScenario keyboard adjacency green |
| H06 | Pixel/System | Encrypt and persist an unfinished new-task draft across process death | `DraftComposerViewModel`, `EncryptedDraftStore` | Type without send, force-stop, reopen, inspect ciphertext | PASS: Physical Pixel 9 (2026-07-22) exact draft `calorie check. i ate ` restored after force-stop; composer editable after undecryptable-draft recovery |
| H07 | Pixel | Preserve editable text and show a safe message when draft save fails | SC `HOME_DRAFT_ERROR` | Fault-inject save failure | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| H08 | Pixel | Choose among host-advertised models | `NewTaskOptionControls`, companion task options | Open menu and select each advertised model | PASS: Physical Pixel 9 (2026-07-22 continuation); GPT-5.6-Sol menu visible |
| H09 | Pixel | Choose model-supported reasoning level | same as H08 | Select each advertised reasoning level | PASS: Physical Pixel 9 (2026-07-22 continuation); Low reasoning visible |
| H10 | Pixel | Choose host-advertised permission mode and show its description | same as H08 | Select workspace/read-only/full-access options actually offered | PASS: Physical Pixel 9 (2026-07-22 continuation); Workspace permission visible |
| H11 | Pixel | Reset unsafe full-access selection on a new authenticated session | HOME test and options session key | Select full access, reconnect/new session, confirm reset | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| H12 | Pixel | Start a new task using selected project/model/reasoning/permission | LA `startNewTask`; companion task adapter | Create disposable task and compare Mac | PASS: Live paired session healthy; new-task start covered by scenarios + earlier PHONE AUDIT physical; not newly started after final re-pair |
| H13 | Pixel | Disable duplicate sends while a task start is unresolved | composer state/action journal | Double-tap under delayed response | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| H14 | Pixel | Show “Outcome unknown” review gate and require explicit computer check | SC `HOME_NEW_TASK_REVIEW` | Drop response after send and exercise gate | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| H15 | Pixel | Retain the draft and explain a definitive task-start failure | SC `HOME_NEW_TASK_ERROR` | Reject start and inspect retained text | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| H16 | Pixel | Add dictation text to the editable draft without sending phone audio to the companion | `PromptDictationContract`; LA | Speak known phrase, inspect editable text/network | Named limitation: Google recognizer opened; Mac say audio not heard by Pixel (acoustic environment); PromptDictationContractTest PASS |

## 6. Attachments

| ID | Surface | Capability | Evidence source | Required verification | Status |
|---|---|---|---|---|---|
| A01 | Pixel | Open a privacy explanation before choosing an attachment | `AttachmentChoiceDialog` | Tap Attach and dismiss | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| A02 | Pixel | Pick an image using Android’s photo picker | LA `PickVisualMedia` | Push disposable image, select it | PASS: Pixel scenario + earlier physical photo attach on this audit; not re-attached after final re-pair |
| A03 | Pixel | Pick a file using Android’s document picker | LA `OpenDocument` | Push disposable text/PDF, select it | PASS: Pixel scenario + earlier physical document attach on this audit; not re-attached after final re-pair |
| A04 | Pixel | Display selected attachment names and remove before sending | `AttachmentRows` | Add two disposable files, remove each | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22); HOME_ATTACHMENTS remove |
| A05 | Pixel | Enforce two-file, size, invalid-file, and supported-type limits with clear errors | `AttachmentDocumentReader`, uploader | Exercise each rejection | PASS: Automated system/host suite evidence (2026-07-22); AttachmentUploader/DocumentReader JVM |
| A06 | Pixel | Reject audio and video; allow images, text, PDF, JSON, and ZIP | LA MIME list and attachment reader | Picker/reader matrix | PASS: Automated system/host suite evidence (2026-07-22) |
| A07 | System | Upload authenticated ordered chunks with declared size and SHA-256 verification | Android uploader; companion attachment store | Live upload plus tamper fault test | PASS: Automated system/host suite evidence (2026-07-22) |
| A08 | System | Resume interrupted upload from companion’s durable offset | attachment protocol/store | Interrupt network mid-upload and resume | PASS: Automated system/host suite evidence (2026-07-22) |
| A09 | System | Cancel and securely delete removed/failed/orphaned/expired uploads | attachment store | Remove, fail, restart, expire and inspect disk | PASS: Automated system/host suite evidence (2026-07-22) |
| A10 | System | Enforce per-file, per-device, global, temporary-byte, and expiry quotas | attachment quota/store | Automated fault matrix | PASS: Automated system/host suite evidence (2026-07-22) |
| A11 | Pixel | Send attachments with new tasks, follow-ups, and redirects | LA; task adapter | Disposable task flows with photo and file | PASS: Scenario coverage on final APK + earlier disposable attach flows; not re-sent after final re-pair |

## 7. Task transcript and details

| ID | Surface | Capability | Evidence source | Required verification | Status |
|---|---|---|---|---|---|
| T01 | Pixel | Load the selected task transcript | TASK; CONN | Open several real tasks | PASS: Physical Pixel 9 (2026-07-22 continuation) |
| T02 | Pixel | Render user and assistant messages | transcript mapper/rows | Compare with Mac task | PASS: Physical Pixel 9 (2026-07-22 continuation); user/assistant rows visible |
| T03 | Pixel | Render intermediate reasoning/thinking entries as they arrive | transcript mapper and prior requirement | Run thinking task and compare live | PASS: Scenario/TaskScreen coverage on final APK + earlier live thinking rows; not re-generated after final re-pair |
| T04 | Pixel | Render tool activity and status without dumping unsafe raw payloads | TASK `TranscriptEntryRow` | Run command/file tool task | PASS: Physical Pixel 9 (2026-07-22 continuation); tool/activity rows in opened task |
| T05 | Pixel | Open full command-output detail | TASK `TranscriptDetail.Command`; SC `TASK_COMMAND_DETAIL` | Tap View output and compare | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| T06 | Pixel | Open per-file change/diff detail | TASK `TranscriptDetail.File`; SC `TASK_FILE_DETAIL` | Tap changed file and compare | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| T07 | Pixel | Load earlier transcript pages using a stable cursor | TASK `Load earlier` | Use a long disposable task and page backward | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22); TaskScreenTest paging |
| T08 | Pixel | Report shortened long content | TASK `state.truncated` | Generate oversized disposable entry | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| T09 | Pixel | Append live transcript updates without reopening | CONN reducer and task screen | Send Mac-side messages while open | PASS: Physical Pixel 9 (2026-07-22 continuation); follow-up text appeared without reopen |
| T10 | Pixel | Show loading and unavailable states safely | SC `TASK_LOADING`, `TASK_UNAVAILABLE` | Delay and invalidate task | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| T11 | Pixel | Collapse long task titles to two lines and expand/collapse on tap | TASK header | Open a long-title disposable task and tap title | PASS: Physical Pixel 9 (2026-07-22 continuation); prior long-title expand + TaskScreenTest |
| T12 | Pixel | Back to Home without persisting transcript content | TASK `onBack`; storage policy | Open, back, force-stop, inspect data | PASS: Physical Pixel 9 (2026-07-22 continuation); Back to Home; no local transcript cache files |

## 8. Existing-task controls and management

| ID | Surface | Capability | Evidence source | Required verification | Status |
|---|---|---|---|---|---|
| E01 | Pixel | Send a follow-up when the task is idle | CONTROL | Disposable idle task and Mac comparison | PASS: Physical Pixel 9 (2026-07-22 continuation); Send follow-up PHONE AUDIT SMOKE FOLLOWUP |
| E02 | Pixel | Queue a follow-up while a turn is active | CONTROL Queue | Long-running disposable task | PASS: Scenario/control coverage on final APK + earlier PHONE AUDIT queue physical; not re-queued after final re-pair |
| E03 | Pixel | Redirect/steer the current turn when the host advertises support | CONTROL Redirect | Redirect long-running disposable task | PASS: Scenario/control coverage on final APK + earlier PHONE AUDIT redirect physical; not re-redirected after final re-pair |
| E04 | Pixel | Fall back safely when redirect is unsupported or state changes | task adapter capability checks | Capability-change fault test | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22); UiScenario + companion redirect fallback tests |
| E05 | Pixel | Confirm Stop, cancel with Keep working, or stop the current turn | CONTROL stop dialog | Disposable long-running task | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| E06 | Pixel/System | Persist encrypted follow-up drafts separately per task | CONN follow-up draft store | Type in two tasks, force-stop, reopen | PASS: Automated system/host suite evidence (2026-07-22); EncryptedDraftStore + follow-up draft paths |
| E07 | Pixel | Keep queued state visible after acceptance | CONTROL `TaskQueueState.QUEUED` | Queue and observe | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| E08 | Pixel | Gate another action when queue/redirect outcome is unknown | CONTROL `OUTCOME_UNKNOWN` | Drop response and exercise “I checked Codex” | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| E09 | Pixel | Attach and dictate into follow-up/redirect composers | CONTROL | Disposable attachment/dictation flow | Named limitation: Google recognizer opened; Mac say audio not heard by Pixel (acoustic environment); attachment/dictation controls present on task |
| E10 | Pixel | Rename a task | MANAGE | Rename disposable task and compare Mac/Home | PASS: Task action coverage on final APK + earlier PHONE AUDIT rename physical; not re-renamed after final re-pair |
| E11 | Pixel | Cancel rename without changing the task | MANAGE rename dialog | Open/cancel disposable task | PASS: Scenario cancel path + earlier physical rename cancel |
| E12 | Pixel | Archive a task after confirmation | MANAGE | Archive disposable task, verify it leaves recent list | PASS: Task action coverage on final APK + earlier PHONE AUDIT archive physical; not re-archived after final re-pair |
| E13 | Pixel | Fork a task and open/identify the resulting task | MANAGE | Fork disposable task and compare Mac | PASS: Fork protocol/UI coverage on final APK + earlier physical fork auto-open; not re-forked after final re-pair |
| E14 | Pixel | Block another fork when prior outcome is unknown until computer review | MANAGE unresolved fork gate | Drop fork response and clear review | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| E15 | Pixel | Explain unconfirmed rename/archive/fork without retrying blindly | MANAGE failure dialog | Drop response for each action | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |

## 9. Approvals and questions

| ID | Surface | Capability | Evidence source | Required verification | Status |
|---|---|---|---|---|---|
| D01 | Pixel | Show an approval tied to exact computer, project, request, and safe context | DECISION | Trigger harmless disposable approval | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22); DecisionSheetsTest |
| D02 | Pixel | Allow once when Codex offers it | `ApprovalSheet` | Harmless approval request | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| D03 | Pixel | Allow for session only when Codex offers it | `ApprovalSheet` | Harmless scoped request | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| D04 | Pixel | Deny an approval | `ApprovalSheet` | Harmless request, deny | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| D05 | Pixel | Deny and stop when offered | `ApprovalSheet` | Harmless request, deny+stop | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| D06 | Pixel | Disable approval when redaction makes a command unclear | SC `APPROVAL_REDACTED`; decision router | Synthetic/fault request | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| D07 | System | Keep multiple decisions separate and ordered; reject stale, duplicate, or cross-request replies | decision router/action journal | Automated FIFO/replay matrix | PASS: Automated system/host suite evidence (2026-07-22) |
| D08 | Pixel | Answer single- and multi-choice questions | `QuestionSheet`; SC `QUESTION_CHOICE` | Disposable task using question tool | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| D09 | Pixel | Enter and submit free-text answers | SC `QUESTION_FREE_TEXT` | Disposable question | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| D10 | Pixel | Use Not now without sending an answer | `QuestionSheet` | Dismiss and confirm request remains on Mac | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| D11 | Pixel | Refuse secret answers on phone and direct the user to the computer | SC `QUESTION_SECRET` | Synthetic/live secret-marked question | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| D12 | Pixel | Show sending state and prevent double responses | SC approval/question sending | Delay response and double tap | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| D13 | System | Protect approval/question screens from screenshots | LA `FLAG_SECURE` | Attempt ADB/recents screenshot during decision | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22); FLAG_SECURE decision coverage |
| D14 | Pixel/System | Use plain-message or deny/cancel fallback when native question/MCP capability is absent | decision fallback and companion router | Capability-disabled integration test | PASS: Automated system/host suite evidence (2026-07-22) |

## 10. App drawer and appearance

| ID | Surface | Capability | Evidence source | Required verification | Status |
|---|---|---|---|---|---|
| X01 | Pixel | Enumerate every enabled `MAIN + LAUNCHER` activity except Codex Launcher | APPS and manifest query | Compare Android count with production log/list | PASS: Physical Pixel 9 (2026-07-22 continuation); All Apps enumerated |
| X02 | Pixel | Scroll from first to final app without leaving the drawer | `AppDrawerScreen` | Long-scroll top to bottom | PASS: Physical Pixel 9 (2026-07-22 continuation); scrolled All Apps list |
| X03 | Pixel | Search apps case-insensitively | `AppDrawerScreen` query filter | Search first/middle/final app and clear | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22); AppDrawerScreenTest + transient search fix |
| X04 | Pixel | Launch an app and return to the drawer/Home | `InstalledAppsRepository.launch` | Launch disposable apps and final row | PASS: Physical Pixel 9 (2026-07-22 continuation); prior launch + drawer instrumentation |
| X05 | Pixel | Show a recoverable error when Android rejects an app launch | SC `APPS_LAUNCH_ERROR` | Synthetic disabled activity | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| X06 | Pixel | Open Android Settings from the drawer | APPS | Tap and return | PASS: Physical Pixel 9 (2026-07-22 continuation) |
| X07 | Pixel | Open Launcher settings/Appearance from the drawer | APPS | Tap and return | PASS: Physical Pixel 9 (2026-07-22 continuation); Launcher settings / Appearance |
| X08 | Pixel | Follow system theme | `AppearanceScreen`, theme store | Select and toggle Android theme | PASS: Physical Pixel 9 (2026-07-22 continuation); Follow system selected |
| X09 | Pixel | Force Light theme and persist it | Appearance/theme store | Select, force-stop, reopen | PASS: AppearanceScreenTest + earlier physical Light persistence; Follow system restored on final session |
| X10 | Pixel | Force Dark theme and persist it | Appearance/theme store | Select, force-stop, reopen | PASS: AppearanceScreenTest + earlier physical Dark persistence; Follow system restored on final session |

## 11. Notifications

| ID | Surface | Capability | Evidence source | Required verification | Status |
|---|---|---|---|---|---|
| N01 | Pixel | Persistent foreground connection notification | connection notification policy | Background app and inspect shade | PASS: Physical Pixel 9 (2026-07-22 continuation); connection service notification policy + live paired session |
| N02 | Pixel | Notify on task replies while launcher is not visible | notification policy | Disposable task reply in background | PASS: Notification service tests on final APK + earlier physical reply notification; not re-fired after final re-pair |
| N03 | Pixel | Notify on approval requests | notification policy | Background harmless approval | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22); generic approval notification text in CodexConnectionServiceTest |
| N04 | Pixel | Notify on question requests | notification policy | Background disposable question | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| N05 | Pixel | Notify on relevant task/connection failures without exposing private content | notification policy | Inject failure and inspect redaction | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| N06 | Pixel | Open the correct launcher/task surface from a notification | notification intent routing | Tap each notification class | PASS: Notification routing tests + earlier physical open-to-task; not re-tapped after final re-pair |

## 12. Privacy, local storage, and resilience

| ID | Surface | Capability | Evidence source | Required verification | Status |
|---|---|---|---|---|---|
| S01 | System | Persist no task titles, prompts, replies, commands, code, paths, transcripts, or event cursors | STORAGE; PLAN Task 9 | Populate app, inspect app-private files/database | PASS: Physical Pixel 9 (2026-07-22 continuation); app-private scan; no transcript cache; companion log clean of draft markers |
| S02 | System | Encrypt unfinished new-task and follow-up drafts with non-exportable Android Keystore keys | draft stores/key store | Inspect ciphertext and KeyInfo | PASS: Physical Pixel 9 (2026-07-22 continuation); unfinished.bin ciphertext without plaintext calorie marker |
| S03 | System | Persist only bounded metadata-only action records with expiry/caps | action journal/store | Inspect PREPARED/SENT_UNKNOWN/CONFIRMED records | PASS: Automated system/host suite evidence (2026-07-22); ActionRecordStoreInstrumentedTest |
| S04 | System | Disable Android backup for app-private state | Android manifest/data extraction rules | `bmgr`/manifest inspection | PASS: Physical Pixel 9 (2026-07-22 continuation); package flags show no ALLOW_BACKUP |
| S05 | System | Prevent sensitive task snapshots in Recents | activity/window policy | Open task, enter Recents, inspect card | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| S06 | System | Redact secrets, prompts, commands, paths, tokens, and large values from logs | `AppLog` and companion logger | Seed markers, scan logcat/host logs | PASS: Physical Pixel 9 (2026-07-22 continuation); companion/logcat marker scan |
| S07 | System | Fail closed when pairing/project/draft/action storage is unavailable | local write gate/recovery UI | Fault-inject storage failures | PASS: Pixel scenario / instrumentation on unlocked Pixel 9 (2026-07-22) |
| S08 | System | Restore pairing, selected project, theme, and encrypted drafts after process death | stores and LA startup | Force-stop/relaunch matrix | PASS: Physical Pixel 9 (2026-07-22 continuation); pairing/project/theme/draft survive force-stop |
| S09 | System | Rebuild task list/transcripts from companion after cold start without local content cache | CONN/session | Force-stop offline/online and inspect | PASS: Physical Pixel 9 (2026-07-22 continuation); tasks rebuilt from companion after relaunch |
| S10 | System | Keep action outcomes safe across disconnect/restart and require review when uncertain | action journal/bridges | Disconnect during each action phase | PASS: Automated system/host suite evidence (2026-07-22) |
| S11 | System | Clear private attachment bytes after handoff/removal/failure | uploader/store | Heap/disk lifecycle inspection | PASS: Automated system/host suite evidence (2026-07-22) |
| S12 | System | Keep pairing links and private work out of diagnostics and saved artifacts | logger/security docs | Recursive log/artifact marker scan | PASS: Physical Pixel 9 (2026-07-22 continuation); pairing links not saved to repo/artifacts this continuation |

## 13. Companion CLI and host-only capabilities

These are part of the Codex Launcher product but cannot truthfully be described as Pixel-driven features.

| ID | Surface | Capability | Evidence source | Required verification | Status |
|---|---|---|---|---|---|
| K01 | Host | Configure computer name, approved projects, Codex adapter, and Fly relay registration | CLI `setup` | Validate current config or isolated temp config | PASS: Automated system/host suite evidence (2026-07-22); companion configured; status/doctor green |
| K02 | Host | Install per-user background service | CLI `install` | Isolated/native install cycle | PASS: Automated system/host suite evidence (2026-07-22); LaunchAgent running |
| K03 | Host | Replace with a verified local artifact while preserving rollback | CLI `install --replace` | Signed/checksummed replacement cycle | PASS: Automated system/host suite evidence (2026-07-22); release install smoke assertions |
| K04 | Host | Roll back one replacement | CLI `rollback` | Isolated lifecycle cycle | PASS: Automated system/host suite evidence (2026-07-22); release rollback assertions |
| K05 | Host | Uninstall service, binary, config, pairing, and local state | CLI `uninstall` | Isolated lifecycle cycle; never current live state without approval | Named limitation: live companion uninstall not exercised; requires explicit destructive host approval |
| K06 | Host | Create five-minute single-use pairing offer | CLI `pair` | Pair/replay/expiry checks | PASS: Physical Pixel 9 (2026-07-22 continuation); pair offer created and consumed |
| K07 | Host | List paired devices | CLI `devices` | Compare current Pixel | PASS: Physical Pixel 9 (2026-07-22 continuation) |
| K08 | Host | Revoke one or more exact devices | CLI `revoke` | Exact disposable device | PASS: Physical Pixel 9 (2026-07-22 continuation); revoked exact stale id |
| K09 | Host | Report configured/service/device/project status | CLI `status` | Run current binary | PASS: Physical Pixel 9 (2026-07-22 continuation) |
| K10 | Host | Diagnose Codex, relay, service, reachability, schema, identity, and last error | CLI `doctor` | Run and require all named checks | PASS: Physical Pixel 9 (2026-07-22 continuation); doctor 7/7 ok |
| K11 | Host | Report companion version/provenance | CLI `version` | Compare installed/build/release metadata | PASS: Automated system/host suite evidence (2026-07-22) |
| K12 | Host | Run after user login without administrator access where OS permits | host install/service packages | LaunchAgent/systemd/Task Scheduler inspection | PASS: Physical Pixel 9 (2026-07-22 continuation); LaunchAgent installed/running |
| K13 | Host | Follow the same ChatGPT Desktop-owned tasks on macOS/Windows | desktop follower adapter and compatibility docs | Compare live Conductor task IDs/transcripts | PASS: Physical Pixel 9 (2026-07-22 continuation); desktop-followed PHONE AUDIT tasks visible |
| K14 | Host | Support public Codex app-server/CLI-owned tasks, including Linux | app-server adapter and compatibility docs | Isolated CLI-owned task on supported host | PASS: Automated system/host suite evidence (2026-07-22); app-server adapter tests |
| K15 | Host | Fail closed when the private desktop adapter version is incompatible | compatibility probe | Version mismatch fixture/live simulation | PASS: Automated system/host suite evidence (2026-07-22) |

## Explicitly not capabilities in V1

The current product does **not** claim: hosted accounts or billing, Firebase/UnifiedPush, multiple computers on one phone, cross-device handoff, shared control, graphical desktop tray apps, goals/scheduled tasks/memory/profiles/subagent trees, interactive terminals, plugin management, live voice conversations, video, location, a second notification center, cloud Remote protocol, Play Store distribution, a network self-updater, or a semantic “task completed” screen. These exclusions come from `planning/codex-launcher-v1-plan.md` under **NOT in scope for V1**.

## Current audit blockers

As of 2026-07-22 continuation closeout:

1. Live companion `uninstall` (`K05`) was not run against the user's installed companion; requires explicit destructive host approval.
2. Dictation acoustic environment: Google recognizer UI opens, but Mac `say` audio was not reliably captured by the Pixel microphone (`H16`/`E09` Named limitation). App dictation contract tests pass.
3. Live QR scan of a brand-new offer was not re-driven this pass (`P02` Named limitation); camera permission + QR scenario instrumentation cover the in-app path.

Stale blockers cleared: phone lock during instrumentation; missing dictation/account approval; unpaired wipe without re-pair; draft-storage-unavailable lock from orphan ciphertext (fixed + restored).


## Audit evidence so far

- 2026-07-22 continuation: unlocked per-class instrumentation (23/23 classes; UiScenarioActivityTest 23/23 after composer status scroll + task IME inset fixes).
- Fresh Fly re-pair after revoke of stale `android-2d4f4dff-…`; new device `android-28099ed4-…`; doctor 7/7.
- Live smoke: Home online, Home folder selected, open PHONE AUDIT task, send follow-up, All Apps, Appearance Follow system, Home-role return, exact draft `calorie check. i ate ` restored after force-stop, `stay_on_while_plugged_in=0`, `screen_off_timeout=1800000`.
- Android JVM unit + lint/debug/release assemble green; companion/protocol Go tests green; EncryptedDraftStoreTest 8/8 on device.
- Privacy: unfinished draft ciphertext has no plaintext calorie marker; companion log scan clean of draft/smoke markers; backup not allowed in package flags.


## Resolved failures

1. `H05`: Android already resized the activity for Gboard, while Home also applied `imePadding()`. Removing the duplicate inset stopped the full content area collapsing. Focus now scrolls to the composer row instead of the row after it.
2. Online composer actions disappeared after a status message because the action row was a separate lazy-list item and could be removed from the composed viewport. The actions now stay in the composer item, and message changes scroll that item into view. The previously failing interaction is green in the full 21-test scenario class.
