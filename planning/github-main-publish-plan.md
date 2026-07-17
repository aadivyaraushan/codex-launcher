# Private GitHub main publish

Date: 2026-07-17

```text
implementation worktree
  ├─ committed project history
  ├─ X-post screenshots and dark composite
  └─ publish-readiness record
              │
              ▼
      final local commit
              │
              ▼
private GitHub repository
aadivyaraushan/codex-launcher
              │
              ▼
        default branch: main
              │
              ▼
privacy + commit + contents audit
```

## Approved result

- Create `aadivyaraushan/codex-launcher`.
- Set visibility to private.
- Preserve the existing git history.
- Include every intended tracked file, the screenshot index, the dark-mode
  composite, and the publish-readiness record.
- Push the implementation tip to remote `main`.
- Make `main` the default branch.

## Steps

1. Confirm the target repository and git remote do not already exist.
2. Commit the remaining intended artifacts on `codex-launcher-v1`.
3. Create the private GitHub repository without generated starter files.
4. Push `HEAD` to `refs/heads/main` and track `origin/main`.
5. Verify GitHub reports `PRIVATE`, default branch `main`, and the same commit
   as the local implementation tip.
6. Verify representative source, Android, companion, documentation, screenshot,
   and saved-result files exist on remote `main`.
7. Ask a fresh reviewing agent to compare the remote evidence with the approved
   result.

## Stop conditions

- Do not create a public repository.
- Do not overwrite an existing remote repository.
- Do not force-push.
- Stop if the remote commit differs after the push or GitHub does not report
  private visibility.
