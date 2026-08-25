# Merge all Codex Launcher worktrees

Date: 2026-08-24

```text
Inventory every worktree
        |
        v
Checkpoint dirty work on its own branch
        |
        v
Merge committed branches into one integration branch
        |
        v
Resolve overlapping product changes by behavior and tests
        |
        v
Run Android + Go + release checks + independent review
        |
        v
Fast-forward main while preserving its pre-existing local files
```

## Done means

- Every live worktree's committed and uncommitted product work is represented in the integration history, or explicitly recorded as already contained, empty, stale evidence, or unsafe generated output.
- No worktree changes are discarded.
- Overlapping files keep the newest compatible behavior rather than choosing a side blindly.
- Android, companion Go, protocol, and release checks pass at the combined head.
- `main` points at the verified combined head; its pre-existing uncommitted files remain present.
- A separate judge reviews the inventory, merge coverage, conflicts, and test evidence.

## Merge order

1. Merge clean committed histories from oldest/base-wide to newest: landing, phase 2, phase 0, OpenAI/Beeper.
2. Checkpoint and merge the large UX routing work.
3. Checkpoint and merge cloud-to-phone updater work.
4. Checkpoint and merge on-device dictation last because it touches current Android composer wiring.
5. Import evidence-only untracked files without generated build directories or nested worktree metadata.
6. Reconcile the current main checkout's own local files before the final fast-forward.

## Safety

- Do not delete or prune worktrees during this merge.
- Do not reset or overwrite dirty worktrees.
- Keep each dirty worktree's checkpoint commit on its existing branch for recovery.
- Do not commit credentials, local secrets, nested `.git` files, or broad generated directories.

## Result

- Integrated the landing page, phase 2 phone tool bridge, routing diagnosis, phone broker, updater, and on-device dictation branches.
- Imported only the four safe cold-reboot health files from the phase 0 checkpoint; the checkpoint with a private SSH key and generated output stayed isolated.
- Kept the phase 2 OpenClaw bridge when the older direct-routing pipeline conflicted with it.
- Ported the older broker's UTF-8 byte reader and `100 Continue` support into the retained generic broker.
- Rebuilt the tracked ARM64 phone runtime from the combined source.
- Go, Android unit/lint/build, release build, and release checks passed before the final review.
