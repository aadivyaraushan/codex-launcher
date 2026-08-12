# Phase 8 — Delete the predetermined-function pipeline

Date: 2026-08-12. Executes Phase 8 of `planning/openclaw-phone-agent-plan.md`
(main checkout, lines 381-395). Goal, observably: no code path left where a
typed request is classified and routed by the old two-stage router; the
agent (via the tool bridge) is the launcher's only brain. Full Go suite and
Android unit tests green; greps for the dead names return zero hits in
code, schema, and scripts.

## Before / after

```
BEFORE  (predetermined pipeline)                AFTER (agent only)

phone Home prompt                               phone Home prompt
      |  capability_request                          |  task message (chat)
      v                                              v
mobilesession handler ──> flow.Service          mobilesession handler ──> task/turn proxy
      |                     |                        |                         |
      |               stage1 (OpenAI route)          |                    OpenClaw agent
      |               stage2 (class resolve)         |                         |
      |                     |                        |                  agentbridge /call
      |                 execution.Runner             |                         |
      |                     |                        |            gates: first-contact / revoke /
      v                     v                        |            irreversible / exfiltration /
capability_preview     adapters                      |            unlisted-send
capability_confirm                                   v
capability_result                               execution.Runner ──> adapters (KEPT)
CapabilitySheet (Kotlin)                        chat UI + approval cards (already built, Phase 6)
```

Kept, untouched in role: registry, adapters, runner, manifests,
consent/approvals, journal, credential broker, contacts graph.

## Two deviations from the master plan's literal bullet list

1. **`capability_disconnect` dies too** (plan names only
   `capability_request`/`capability_confirm`). Its ONLY sender is the
   Kotlin CapabilitySheet result view (`CapabilityInteraction.kt:310`,
   `LauncherActivity.kt:925`), and that sheet only ever appears for
   preview/result frames the deleted pipeline produced — keeping the wire
   kind keeps a dead path, which the fix-in-place rule forbids. The
   behavior (owner can disconnect an app) moves to the agent: see Unit 1.
2. **`routing/eval` and the 11 `*_proof.go` commands + 12 single-adapter
   `runtime.New<X>` builders are deleted** (plan doesn't name them). All of
   them exist only to construct or exercise stage1/stage2/flow and cannot
   compile once those are gone. The proof commands were scaffolding for
   already-passed phase proofs; their evidence lives in `saved-results/`.

## Units, in order (each ends green before the next starts)

### Unit 1 — Agent-side disconnect replaces the wire kind (Go, TDD)

New behavior FIRST so the deletion never drops the ability to disconnect.

- `agentbridge`: the tool list gains a `disconnect` verb per connected
  adapter; a `/call` with verb `disconnect` sets `CallFacts.Revoke: true`
  → always gated (`KindRevoke`, policy already built and tested in
  Phase 4). On approval the bridge runs the revoke through a new
  `capability/disconnect` package: `runner.Revoke` (credentials first)
  then consent-store revoke, idempotent on repeat — a port of
  `flow.Service.Disconnect` (`flow/service.go:231-275`) minus the
  pending-preview scan (previews no longer exist).
- Red tests: bridge test — `disconnect` call gates with `KindRevoke`,
  approval revokes exactly once, repeat disconnect succeeds without
  reaching the registry; port the still-meaningful cases of
  `flow/disconnect_test.go` onto the new package.

### Unit 2 — The Go deletion wave (one commit, mechanical after Unit 1)

Delete outright:
- `companion/internal/capability/routing/stage1/` (10 files, 3421 lines,
  incl. the 5 TestBrokered* that fail on Mac today),
  `routing/stage2/` (5 files), `capability/flow/` (4 files),
  `routing/eval/` (imports both, orphaned).
- `companion/cmd/codex-launcher/*_proof.go` (11 files) and
  `runtime/{youtube,maps,microsoft,google,podcasts,todoist,slack,spotify,
  msteams,notion}.go` + `runtime/instagram/` + `runtime/deeplink/` flow
  builders, plus their tests.
- In `runtime/production.go`: `classAddressing` (:56-76), `classMapFor`
  (:87-93), the `stage2.New`/`stage1.New`/`flow.New` tail (:484-499).
  `NewProduction` becomes the tool-surface builder: registry + adapter
  registration + runner + consent + Inventory, no `Model` config, return
  type loses `*flow.Service`. Its class-map tests
  (`production_classmap_test.go`, `beeper_stage2_readonly_test.go`,
  `notification_reply_reachable_test.go`,
  `every_class_is_routable_test.go`) die with the concept.
- `cmd/codex-launcher/production.go`: `productionRoutingModel` and the
  stage1 imports go; `startProductionCapabilityFlow` shrinks to whatever
  the desktop serve still needs (task surface only).
- `phoneruntime/runtime.go`: the `stage1openai.NewBrokered` block
  (:263-271) goes; `Dependencies.Flow` and `handler.EnableCapabilities`
  go; the bridge keeps being built from `NewProduction`'s Inventory —
  now unconditionally, not only when `flow == nil`. `routerStatus`/
  `routerSource` surface: delete with the router; the gateway connection
  is the health signal (implementer verifies consumers before deleting).
- `mobilesession/handler.go`: `capability_request`/`capability_confirm`/
  `capability_disconnect` branches, `handleCapabilityAction`,
  the async dispatch gate at :717-753 (the `if action.Kind ==
  "capability_request" || ...` block whose goroutine exists to keep slow
  capability calls off the websocket read loop — dies whole; the task
  path never used it), the `CapabilityFlow` interface, `capabilityflow` +
  `stage1openai` imports, and the capability failure-code mapping that
  only they used. Tests that drove them (`capability_*_test.go`,
  `router_fail_closed_test.go`, `disconnect_*_test.go`) die or migrate
  to Unit 1's package.
- Handshake, designed not accidental: the `welcomeBody(...)` call at
  handler.go:525 passes `handler.capabilityFlow != nil`; with the field
  gone, delete that boolean parameter end to end so the `capabilities`
  array simply never contains `"capability_actions"`. An old phone reads
  `capabilityActionsCapable == false` and uses its existing
  send-to-paired-computer fallback (`LauncherSessionViewModel.kt:485-494`)
  — wire compatibility is this designed behavior, not however a compile
  error gets resolved.

### Unit 2.5 — Agent-side device hand-off (Go, TDD) — added after Unit 2 landed

Unit 2 exposed a false premise in this plan's risk list: device work
(notification replies, YouTube playback) was NOT task-path — its only
producer was `flow` → `handOffToDevice`, both deleted. After Unit 2,
nothing catches `adapter.DeviceWorkError`, so the `notification_reply`
and `youtube` adapters (kept surface — "adapters are the tool surface
now") fail opaquely through the bridge, and the mobilesession receiving
side runs on an always-empty ledger. The behavior moves to the agent
path, same principle as Unit 1:

- `devicework.Ledger` becomes a rendezvous, not a poll target:
  `Wait(record)` returns a result channel (refused if the request id is
  already outstanding); settling delivers to that channel; `DeviceGone`
  fails every waiter of that phone. `Expired` and the ledger's
  timeout/clock die — the waiting caller owns its own deadline.
- `mobilesession.Handler` gains `RunOnDevice(ctx, ask)`: no connected
  phone → error without touching the ledger; otherwise register, send
  the same `device_action` wire frame as before, and block until the
  phone's `device_action_result`, the ctx deadline, or the phone
  disconnecting — deadline/disconnect answer outcome-unknown, never
  "failed" (a reply that timed out may already sit in someone's chat).
- `handleDeviceActionResult` keeps the outcome-word mapping and the
  hands_off done-clamp, but delivers to the waiter instead of
  journaling `capability_result` — killing that frame's last real
  producer (Unit 3's premise becomes true). `SweepDeviceWork`,
  `publishCapabilityActionResult`, `publishCapabilityFailure` lose
  their last callers and die.
- `agentbridge`: a `DeviceWorker` dependency next to `Disconnector`;
  `executeCall` catches `*adapter.DeviceWorkError` → `RunOnDevice`
  under `mobilesession.DeviceWorkTimeout`; result maps onto
  `ToolCallResult` (reached = adapter's declared ceiling, done, detail).
  Nil DeviceWorker → `adapter_failed` naming the missing phone.
- `phoneruntime/runtime.go` wires the handler in as the bridge's
  DeviceWorker.

### Unit 3 — Protocol prune, both sides in lockstep (TDD on contract tests)

- `protocol/schema/action.schema.json`: remove the three capability kind
  branches (:90, :100, :112).
- `protocol/schema/envelope.schema.json`: the `capability_preview` and
  `capability_result` FRAME types die with their only producer
  (`handleCapabilityAction`) — remove them from the type enum (:20, :26)
  and delete both `if/then` schema branches (:186-229).
- Go `mobileapi/contract/validation.go`: the action-kind branches
  (:1026-1040) AND the envelope-type switch cases for
  `capability_preview`/`capability_result` (:574, :581);
  `contract_test.go` / `device_action_test.go` fixtures for all five
  names out. An incoming capability kind now fails schema validation
  like any unknown kind.
- Kotlin `ProtocolCodec.kt` (:250-260) branches out;
  `ProtocolContractTest.kt` fixtures out.

### Unit 4 — The Kotlin deletion wave

- `capability/interaction/` (CapabilityInteraction.kt 663 lines,
  CapabilitySheet.kt), `storage/capability/unresolved/`, the
  `CapabilitySheet(...)` block in `LauncherActivity.kt` (:911-927),
  `capabilityController`/`capabilityInteraction` wiring in
  `LauncherSessionViewModel.kt`, `PromptDestination` and the Home
  destination toggle (`HomeScreen.kt`, `HomeUiState.kt`,
  `HomeSendRouter.kt`): every Home prompt now goes down the task path
  (`startNewTask`), which is the agent chat since Phase 6.
- `ScenarioCatalog.kt` CAPABILITY_CONFIRM scenario and the capability
  cases in `LauncherSessionViewModelTest`, `CapabilityInteractionTest`,
  `CapabilitySessionLostTest`, `CapabilityDraftRetentionTest`,
  `ScenarioCatalogTest`.
- The `capabilityActionsCapable` handshake flag: remove the phone's use;
  hello/capability negotiation stays wire-compatible (flag simply unused).
- Survey deltas (read-only sweep, 2026-08-12) — the map above missed:
  - `capability/handoff/HandOffActions.kt` dies (only callers:
    CapabilitySheet + LauncherActivity:924). `handoff/youtube/`,
    `capability/reply/`, `capability/notifications/` are UNRELATED
    features that survive — a name-based sweep must not catch them.
  - `CapabilityOutcome.kt` splits: `CapabilityOutcome`/`Ceiling`/
    `CapabilityBadge`/`toTaskState()` die; `StateMark`/`MarkShape`/
    `MarkFill`/`MarkTone`/`StateMarkView` are shared design-system
    primitives (TaskSummary, HomeScreen, ThreadAskCard) — keep, move
    out of the capability package. `TaskQueueState.OUTCOME_UNKNOWN` and
    `TaskState.UNVERIFIED` are general task infra driven by the wire —
    they stay.
  - Wipe pipeline: `LocalStateWiper.kt` WipeStep.CAPABILITY_UNRESOLVED_CHECK
    (:27, :43, :158 param, :174) + `LocalStateOwner.kt` (:7-8, :40, :64)
    + `LauncherApplication.kt:104` DI param +
    `LocalStateWiperInstrumentedTest.kt` (positional 12-arg fromStores
    call — edit in lockstep).
  - The phone's on-disk `unresolved_capability_check` DataStore file is
    orphaned after deletion — inert, one short sentence, no reader left;
    accepted as harmless (no migration).
  - ScenarioCatalog has SIX capability entries (:55, :60-64), not one;
    UiScenarioActivity's ReplyConsentScenario is shared (targeted edit),
    CapabilitySheetLaterPhaseScenario dies. `ScenarioCatalogTest`
    exhaustive set literal must drop the 6 wire names.
  - Shared tests needing targeted edits (not deletion):
    `LauncherSessionViewModelTest` (5 tests + helper; the
    `missingProjectCapability*` test is a false positive — different
    "capability", stays), `HomeUiStateTest` (7 promptDestination args),
    `HomeScreenTest`, `UiScenarioActivityTest`,
    `CompanionSessionClientTest` (uses CAPABILITY_RESULT as a stand-in
    replay frame — re-point at a live frame type since the protocol
    prune removes it), `HomeUnresolvedRowTest`/`HomeTaskMarkTest` only
    shift if StateMark's package moves.

### Unit 5 — Orphan sweep + full matrix (the wave's proof)

- `grep -rn "capability_request\|capability_confirm\|capability_disconnect\|capability_preview\|capability_result\|stage1\|stage2\|classAddressing\|classMapFor\|capabilityflow\|CapabilitySheet\|CapabilityInteraction\|PromptDestination"`
  across Go, Kotlin, schema, docs, release checks — zero hits outside
  `planning/` and `saved-results/` history, EXCEPT the deliberate
  negative fixtures that prove rejection: the two Unit 3 regression
  tests (`contract_test.go`, `ProtocolContractTest.kt`) and
  `protocol/fixtures/invalid/schema-drift.jsonl:41` (still rejected,
  now as an unknown kind).
- `go test -count=1 ./companion/...` — fully green INCLUDING the openai
  package deletions (today's 3 Mac-only TestBrokered* failures disappear
  with stage1; a green full suite on the Mac becomes possible for the
  first time and is the required end state).
- Android: `./gradlew :app:testDebugUnitTest` green (baseline 807/0
  adjusted down by deleted tests, 0 failures).
- Judge: fresh-context judge derives its own checklist from the master
  plan's Phase 8 bullet + this file, then attacks the diff.

## Risks the implementer must not discover mid-flight

- `mobilesession/handler.go` interleaves kept failure-code mapping
  (`consent.ErrNotGranted`, `execution.ErrVerbNotOffered`) with doomed
  branches in one switch — prune surgically; check `capabilityFailureCode`
  for remaining callers before deleting the whole function.
- `phoneruntime/runtime.go` is 1200 lines with the doomed
  `NewBrokered` block easy to miss — grep after edit.
- `deeplink` ADAPTER stays (it is a tool); only `runtime/deeplink/`
  (the flow builder) dies. Same distinction for `instagram`.
- Kotlin `DeviceReplyRequest.kt` doc comments reference
  `capability_confirm` — comments updated; the device-work behavior
  itself survives via Unit 2.5 (this bullet originally claimed it was
  "task-path and stays", which Unit 2 proved false — the flow path was
  its only producer).
- `contacts` package stays (used beyond stage2).
