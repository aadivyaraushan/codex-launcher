# Operator autonomous implementation — August 1

```text
Approved Operator scope
        |
        v
Isolated git worktree
        |
        v
Tests first -> implementation -> full checks
        |
        v
Readiness report + independent review
        |
        v
Stop at any user-only external gate
```

## Observable done

- The worker implements the approved Operator capability scope in an isolated worktree.
- New or changed behavior has tests written first; the red test run is recorded before implementation and the green run after it.
- Go, Android, release, and integration checks listed in `README.md` run or are clearly marked unavailable, with the exact failure saved.
- Diagnostic logs explain inputs, decisions, outputs, and caught errors without secrets or raw user speech.
- A readiness report states what is good to go, what remains blocked, and the exact user action for each external gate.
- No credentials are stored in files or logs. No final Play submission, public release, payment, invitation, or message is completed without its required user gate.

## Work sequence

1. Re-read the repository state and preserve existing untracked files.
2. Create a separate `codex/` git worktree before any implementation edit.
3. Inspect the capability contract, current tests, release checks, and existing planning documents.
4. Convert each approved gap into a failing test at the level where it can break: unit, integration, Android, or release check.
5. Implement the smallest changes that make the tests pass; keep the current direct-route or official-app handoff policy.
6. Run the complete available test and build matrix, including race checks, vet, Android unit/lint checks, release checks, and device checks when a device is available.
7. Recheck sibling paths for the same contract or logging issue.
8. Produce a readiness report and have a separate judge review the result against the task from first principles.

## External gates

```text
Code/build/test work ------------------------------> worker may complete
Play account, real account login, device permissions -> stop for user gate
Billing caps or paid calls -------------------------> stop unless explicitly approved
Play upload, publish, tester contact, final Submit --> stop for user review
```

The worker may prepare exact instructions and open a user-controlled handoff, but it must not claim an external action succeeded without post-action verification.

## Current repository evidence

- Repository: `/Users/aadivyar/Documents/Startups/ai native mobile software/codex-launcher`
- Branch at investigation: `main...origin/main`
- Existing untracked files: `.claude/`, `saved-results/claude-desktop-remote-control-disconnect-bug.md`, `saved-results/operator-agent-billing-account.md`, and `saved-results/wave0-test-accounts-setup.md`; preserve them.
- Baseline checks are documented in `README.md`: Go tests with race detection, `go vet`, Android unit tests and lint, release checks, and optional connected Android tests.
- The earlier Operator plan says AI handles implementation, testing, fixes, documentation, and drafts; the owner handles decisions and external gates.

## Handoff format

- Worktree path and branch.
- Files changed and why.
- Red and green test commands with exit codes.
- Checks skipped and the plain reason.
- Remaining blockers and user-controlled next actions.
- Independent judge result.
