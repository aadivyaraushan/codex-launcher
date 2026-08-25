# Phase 6 — Home thread-list slice (judged PASS, committed)

Date: 2026-08-12
Commit: 4fcd887 (worktree branch worktree-phase2-tool-bridge)
For: the chat-first UI collapse (Phase 6 of planning/openclaw-phone-agent-plan.md), Home screen slice after slices 1/2a/2b (3057d8e, 0ecb71c, 3ca3a97).

## What was built

- Snapshot protocol: optional per-task `lastMessage {from: agent|user|plain, text}`, strictly validated on both sides (exact keys, known speakers, safe display text capped at 512 chars). Sources: app-server thread `preview` (plain), turn-proxy send/steer (user), agent reply (agent). Reply events also update the row live on the phone without a resync.
- Home rows: last-message preview ("Agent: …" / "You: …") under the title; every ordinary lifecycle state now carries its state mark (WORKING, WAITING_FOR_USER for both waiting states, REPLIED; INTERRUPTED alone stays bare — DESIGN.md "words plus shape").
- Ordering: `sortedForHome()` — tasks waiting on the user sort first regardless of recency, then most-recent activity.
- New-session flow: the destination segmented control is deleted; two buttons "on phone" / "on computer" replace it. Project pick + Model/Reasoning/Permission controls render only inside the computer flow. PromptDestination enum unchanged (AUTO rename waits for Phase 8).
- Ask cards in the thread carry the WAITING_FOR_USER mark ("Approval needed" / "Needs your answer").

## How it was verified

- TDD: 6 red tests written first (Go: taskstate mapper, turnproxy source, mobilesession handler, contract validation; Kotlin: ProjectSessionBridge, TaskEventReducer) plus 3 Home red tests (marks, preview, ordering). Red confirmed (compile failures + contract rejections), then Sonnet subagents implemented A (data path) and B (screen).
- Suites: Go `go test ./...` green except the 3 pre-existing Mac-only TestBrokered* failures in stage1/openai (not ours). Android: 807 unit tests / 0 failures (baseline was 803).
- Fresh judge (independent context, checklist derived from plan + DESIGN.md + mockup before seeing the diff): first verdict FAIL over one stale test (HomeUnresolvedRowTest still asserting the old null mark for WORKING — a failure my own gradle run had missed); fixed, judge re-verified 807/0 itself. Final VERDICT: PASS.

## Open follow-ups

- ~~Phone-agent task loses its `lastMessage` across a gateway redial~~ — FIXED 2026-08-12 (commit after 4fcd887): `turnproxy.Config` gained `InitialLastMessage`; `Connect` seeds the fresh Source with it; the runtime's redial loop reads the dropped source's last message (CurrentTask, under the source's lock, after Done fires so no writer races) before closing it and passes it into the next dial. First dial passes the blank zero value, and a failed read keeps the previous carried value. TDD: 2 red tests first (Connect seeding; second dial's config carries the first source's message), then implementation by a Sonnet subagent; fresh judge derived a 6-point checklist first, found no gaps, re-ran the suite itself with `-race` (113 passed / 0 failed) — VERDICT: PASS.
- On-device Pixel proof for this slice (hard-gated send arrives as a chat message; typing "go ahead" does NOT release it, tapping Approve does) — blocked on the phone being unlocked (PIN). Screen-driven adb only; evidence to saved-results/.

## Reproduce

From the worktree root:
- Go: `cd companion && go test ./internal/codex/taskstate/ ./internal/phoneruntime/turnproxy/ ./internal/app/mobilesession/ ./internal/mobileapi/contract/`
- Android: `cd android && ./gradlew testDebugUnitTest` then sum `tests=`/`failures=` across `app/build/test-results/testDebugUnitTest/*.xml`.
