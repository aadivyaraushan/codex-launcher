# Wave 1 Microsoft Teams work/school Graph OAuth — judge

**Date:** 2026-08-02  
**Worktree:** `phase0-notification-probe`  
**Subject:** `saved-results/wave1-msteams-work-oauth.md` + adapter / OAuth / proving code  
**Judge role:** Independent LLM-as-judge. Fresh context. Standard defined from first principles first; no implementer checklist. No product edits.

Docs re-checked (Context7 `/websites/learn_microsoft_en-us_graph`, 2026-08-02): `GET /me/chats` and chat message APIs are work/school delegated; personal Microsoft accounts are not supported for chat message retrieval. Send uses `POST /chats/{chat-id}/messages`; delegated work/school least privilege includes `ChatMessage.Send`, higher `Chat.ReadWrite`.

---

## Standard (what strong looks like)

**Inputs → Outputs → Algorithm** for a strong Wave 1 work/school Teams Graph slice:

### Inputs
- Work/school user OAuth (not personal MSA).
- Intent to **read** or **send** in an existing chat (subject = chat topic or id; body = message text for send).
- Separate personal Teams path that must keep working as compose hand-off.

### Outputs
- Adapter id `msteams` (not `teams`) with ceiling **completes** for read+send.
- Authorize URL that requests **Chat.ReadWrite** (or an equivalent pair that actually covers list+send) against an **organizations / work tenant**, never `consumers`.
- Outlook personal mail OAuth still: mail scopes only, `consumers` (or `MICROSOFT_TENANT`) via `Start`, unchanged.
- Personal deeplink id `teams` still compose-only prepare-and-open.
- Evidence that is honest about what was proven live vs unit-only.

### Algorithm (must be true for “strong”)
1. **Split by identity and product path** — work Graph vs personal deeplink are different ids, different auth, different ceilings; one cannot silently become the other.
2. **Auth is hard to misuse** — the Teams authorize entrypoint forces (or clearly fails closed on) work/school tenant + chat scopes; mail entrypoint cannot pick up chat scopes by accident.
3. **API shape matches Graph** — list chats / post message with Bearer token; no claim that personal MSA Graph chat works.
4. **Read+send “completes” means the user-visible job finishes** — send posts after preview; read returns chat content that was actually fetched (or the product explicitly scopes “read” as resolve-only and does not overclaim).
5. **Safety rails** — empty send body rejected; ambiguous chat match asks; revoke clears token; logs tagged, no secrets.
6. **Isolation proven by tests** — mail scopes/tenant path green; deeplink `teams` still present; `msteams` ≠ `teams`.
7. **Operable proof path** — owner can start Teams authorize without rewiring Outlook’s serve path; live Approve may remain gated, but the wire to Approve exists.

---

## Verdict

**PASS WITH GAPS** — strong enough as a **unit-proven adapter + OAuth split** for overnight Wave 1; **not yet strong end-to-end** against the first-principles bar above.

The core contract (separate `msteams`, `Chat.ReadWrite`, keep Outlook `consumers` mail and personal `teams` deeplink) holds in code and tests. Weaknesses are tenant fail-closed, missing product wiring/CLI, and a thin Read that resolves a chat without fetching messages.

---

## Findings

### What holds (verified)

| Claim | Evidence |
|---|---|
| Distinct adapter id `msteams` vs deeplink `teams` | `adapters/msteams/adapter.go` `ID = "msteams"`; deeplink catalog still `{ID: "teams", … Compose …}` in `adapters/deeplink/adapter.go:72`. Manifest test refuses id collision (`msteams_test.go`). |
| Read+send completes ceiling, OAuth RT-2 | `Describe()`: `RT2`, `Completes`, `AuthOAuth`, verbs Read+Send only. |
| Chat scopes separate from mail | `ChatScopesForVerbs` → `offline_access User.Read Chat.ReadWrite`. `ScopesForVerbs` unchanged mail set; test `TestChatScopesForTeamsVerbsLeaveMailScopesUnchanged` asserts no cross-contamination. |
| Send posts Graph message after preview | Client `POST /v1.0/chats/{id}/messages` with `{body:{content}}`; execution test requires preview then one post (`msteams_test.go`). |
| List uses documented chats endpoint | Client `GET /v1.0/me/chats` with `$select=id,topic,chatType` (`client.go`). |
| Outlook serve path untouched | `cmd/codex-launcher/microsoft_proof.go` still defaults tenant `consumers` and calls `msproof.Authorize` (mail `Start`), not `AuthorizeChat`. Runtime `NewMicrosoft` still registers Outlook only. |
| Personal Teams routing coach intact | Stage1 test still expects instructions to mention personal Teams compose / prepare-and-open (`routing/stage1/openai/client_test.go:187`). |
| Unit suite green | Ran `go test ./companion/internal/capability/adapters/msteams/... ./oauth/microsoft/ ./proving/microsoft/ -count=1` → **17 passed**. Outlook/deeplink packages also green in a follow-on run. |
| Evidence doc honesty | Open items correctly list owner Approve, optional `serve-msteams-proof`, live send not overnight-required. |

### Gaps vs the strong bar

1. **`StartChat` does not fail closed on tenant.**  
   Comment says callers must set `organizations`; implementation reuses whatever `Flow.Tenant` / authorize URL `New` baked in. Judge probe (temporary local test, deleted after): `New(Tenant: "consumers")` + `StartChat` → authorize path still `/consumers/` with `Chat.ReadWrite`. So the Teams entrypoint can emit a known-bad personal-tenant chat authorize URL. Tests only cover the happy path where tenant is already `organizations`.

2. **No companion runtime / CLI wire for `msteams`.**  
   Grep: no `msteams` under `capability/runtime` or `cmd/codex-launcher`. `AuthorizeChat` exists in proving, but `serve-microsoft-proof` remains Outlook-only; no `serve-msteams-proof`. Owner cannot exercise the Teams path through the same operable serve surface as Outlook/Slack/Todoist without new wiring (evidence lists this as open — accurate).

3. **Read “completes” without reading messages.**  
   `Execute(Read)` returns topic + chat id from resolve; no `GET /chats/{id}/messages` (or equivalent). Resolve itself only lists chats. Against Outlook’s peer (list messages + subject/preview in the plan), this is a thinner “read.” Evidence scopes read as list+resolve; that is narrower than a plain-language “read completes” for chat.

4. **Resolve quality limits.**  
   - Only first page (`$top=50`), no `@odata.nextLink` follow.  
   - 1:1 chats often have empty `topic`; match then requires knowing Graph chat id.  
   - Partial topic match is substring-only; no member-name resolve.

5. **Proving test’s organizations assertion is soft.**  
   `TestAuthorizeChatStartsWithChatScopes` checks `/organizations/` in output because the **fake** `StartChat` hardcodes that URL — it does not prove a real `Flow` with wrong tenant is rejected.

6. **Live bar still open (acknowledged).**  
   Live start test skips without env; owner Approve / live send not done. Unit green ≠ Graph chat send proven against a real work tenant.

---

## Isolation check (must-not-break)

| Path | Status |
|---|---|
| Outlook personal mail OAuth (`consumers` / `Start` / mail scopes) | **Intact** — serve + runtime + `ScopesForVerbs` unchanged; isolation tests pass. |
| Personal deeplink `teams` | **Intact** — still in deeplink catalog as compose; stage1 personal compose coach still tested. |
| Cross-scope bleed | **Not observed** — chat scopes helper does not request mail; mail helper does not request `Chat.ReadWrite`. |

---

## Gaps (summary list)

1. Tenant not enforced inside `StartChat` (misconfig → `consumers` + `Chat.ReadWrite`).
2. No `msteams` runtime registration or serve CLI.
3. Read does not fetch message content.
4. Chat list pagination / empty-topic 1:1 resolve unsolved.
5. Live work-tenant Approve + send still owner-gated.
6. Stage1/router not shown teaching work Teams → `msteams` (personal compose path still the instructed default for “Teams”).

---

## Next tip

Make `StartChat` **fail closed**: if tenant is empty/`consumers` (or authorize URL path contains `/consumers/`), return an error and refuse to build the URL — then add a unit test that default-tenant `StartChat` fails. That one change closes the highest-severity hole before wiring `serve-msteams-proof`.

---

## Commands / how to re-judge

```bash
cd "/Users/aadivyar/Documents/Startups/ai native mobile software/codex-launcher/.claude/worktrees/phase0-notification-probe"

go test ./companion/internal/capability/adapters/msteams/... \
        ./companion/internal/capability/oauth/microsoft/ \
        ./companion/internal/capability/proving/microsoft/ -count=1

# Isolation smoke (optional)
go test ./companion/internal/capability/adapters/outlook/... \
        ./companion/internal/capability/adapters/deeplink/ -count=1
```

Re-read: evidence `saved-results/wave1-msteams-work-oauth.md`; code under `adapters/msteams/`, `oauth/microsoft/flow.go` (`ChatScopesForVerbs` / `StartChat`), `proving/microsoft/proof.go` (`AuthorizeChat`), `cmd/.../microsoft_proof.go`, deeplink `teams` row.
