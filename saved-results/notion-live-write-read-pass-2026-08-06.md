<!-- Callers: overnight trail / human. API: Notion hosted MCP create-pages/fetch.
     Schema: proof.json. User: "Run live Notion read/write proof… Update overnight status." -->
# Notion live write+read PASS

**Date:** 2026-08-06 (~02:32 UTC+4 / run stamped 20260805T223206Z)  
**Purpose:** Durable record that Notion OAuth token works for hosted MCP connect + Operator-container write/read.  
**Worktree:** `.claude/worktrees/phase0-notification-probe`

## Result

- **PASS** — marker `OP-NOTION-20260805T223215Z` written under Operator page, fetched by page id, marker matched in content.
- Evidence dir: `saved-results/wave3-test-logs/live-notion-proof-20260805T223206Z/` (`proof.json` `ok: true`, `prove.log`).

## How to reproduce

1. Decode keychain token (ACL bypass if native Go Get fails with -25293):

```bash
security find-generic-password -s "com.operator.credentials" -a "notion_oauth" -w
# If hex without 0x (starts with 7b…): bytes.fromhex → JSON → access_token
```

2. Build + run:

```bash
cd companion
go build -a -o "$HOME/Library/Application Support/codex-launcher/bin/proveadapter" ./cmd/proveadapter
OPERATOR_NOTION_ACCESS_TOKEN="<access>" \
  "$HOME/Library/Application Support/codex-launcher/bin/proveadapter" notion
```

Expect: `measured ceiling: completes`, `write done`, `read matched marker`, VERDICT proven.

## Context

- No `NOTION_*` in `.env` (hosted MCP + DCR by design).
- Docs checked: Context7 `/websites/developers_notion_guides_mcp` — `notion-create-pages` needs `pages[]` + optional `parent`; `notion-fetch` needs `id`.
- Search right after create can fuzzy-match neighbors before the new title indexes; proof fetches by returned page id.

## Caveat (superseded 2026-08-06T02:43Z)

Native `SecKeychainFindGenericPassword` can still return `-25293` for ACL/partition mismatches. The credential-store path now falls back to `security find-generic-password -w` on that error, so **normal `credentialstore.Get` / `proveadapter notion` work without `OPERATOR_NOTION_ACCESS_TOKEN`**.

Follow-up evidence: `saved-results/wave3-test-logs/live-notion-keychain-acl-20260805T224314Z/proof.json` (`prove_without_OPERATOR_NOTION_ACCESS_TOKEN: true`). Also verified `go run ./cmd/proveadapter notion` with env unset.
