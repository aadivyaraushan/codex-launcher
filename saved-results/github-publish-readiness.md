# GitHub publish readiness

Date: 2026-07-17

## Purpose

Prepare the complete Codex Launcher worktree for a verified push to a GitHub
repository without publishing before the owner, name, and visibility are
explicitly approved.

## Verified local state

- Working repository:
  `/Users/aadivyar/Documents/Codex/2026-07-12/uf-u-implementation`
- Current implementation branch: `codex-launcher-v1`
- Current committed tip before the final publish commit:
  `4c96cea529b0f5a81a64bb44aa4a9e76ace2d042`
- GitHub CLI account: `aadivyaraushan`
- Proposed repository: `aadivyaraushan/codex-launcher`
- Approved visibility: private
- The proposed repository name did not exist when checked with `gh repo view`.
- There was no existing git remote.
- The repository had 441 tracked files and 80 tracked files under `outputs/`.
- The tracked `outputs/` directory used 6.7 MB.
- All commits use the GitHub private-address author
  `96186374+aadivyaraushan@users.noreply.github.com`.

## Remaining intended work

- `outputs/x-post/dark-mode-four-screen-composite.png`
- `saved-results/x-post-screenshot-set.md`
- This readiness record

The dark-mode composite is 1024 x 1536. Its SHA-256 is:

```text
9968d51f1c036f784421280b048728ffef4efda3fd1d2de8edef1f7ead113496
```

## Public-safety scan

The current files and every reachable commit were checked for common GitHub,
AWS, OpenAI, Slack, Google API, and private-key patterns. The apparent `sk-`
matches were verified as false positives from filenames such as
`task-14-public-alpha-release-checkpoint`. No literal one-time pairing URI
containing a 22-character secret was found in reachable history. No
secret-like tracked filenames were found.

The scan is evidence for the listed patterns, not a guarantee against every
possible secret format.

Several saved-result documents intentionally contain the local username and
absolute paths under `/Users/aadivyar`. Publishing the complete worktree will
make those paths public unless the user chooses to sanitize or exclude them.

## Publish steps after approval

1. Record the approved owner, repository name, and visibility.
2. Write or update `planning/github-main-publish-plan.md`.
3. Commit the remaining intended artifacts.
4. Move the implementation branch to local `main` without losing history.
5. Create the GitHub repository without an extra generated README or license.
6. Push local `main` and set its upstream.
7. Verify the remote URL, visibility, default branch, remote commit, and key
   repository files through GitHub.
8. Have a fresh reviewing agent compare the verified remote state to the
   publish objective.

## Reuse

Before a later publish, rerun:

```sh
git status --short --branch
git remote -v
gh auth status
gh repo view OWNER/REPOSITORY --json nameWithOwner,visibility,url,defaultBranchRef
```
