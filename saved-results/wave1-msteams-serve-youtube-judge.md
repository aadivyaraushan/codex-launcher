# Wave 1 judge: `serve-msteams-proof` + YouTube prepare-and-open

**Date:** 2026-08-02  
**Worktree:** `phase0-notification-probe`  
**Role:** Independent LLM-as-judge (fresh context). Standard defined from first principles before grading. No implementer bug checklist. No product edits.  
**Subjects:** `saved-results/wave1-msteams-work-oauth.md` + `saved-results/wave1-youtube-prepare-open.md` + related code.

## Gate facts (why this file)

1. **Callers:** None in code. Human/parent-agent artifact under `saved-results/`. Peer pattern: other `wave1-*-judge.md` files.
2. **Prior related judgment:** `wave1-msteams-work-oauth-judge.md` graded the earlier adapter/OAuth split and found **no serve CLI** and **no consumers fail-closed**. This judgment covers the later serve-wiring + fail-closed work, plus YouTube Wave1Specs→39. It does not re-litigate thin Read / pagination from the prior msteams judge.
3. **Data I/O:** None — static markdown verdict.
4. **User instruction:** Independent LLM-as-judge; define strong from first principles; grade both deliverables; write this one file dated 2026-08-02; Pass|Pass-with-warnings|Fail; no product edits.

**Code reviewed:**  
- Teams serve: `cmd/codex-launcher/{main,msteams_proof,microsoft_proof}.go`, `oauth/microsoft/flow.go`, `proving/microsoft/proof.go`, `runtime/msteams.go`, adapter package  
- YouTube: `adapters/deeplink` Wave1Specs, `runtime/deeplink`, stage1 OpenAI coaching, `HandOffActions` (+ unit test source), `deeplink_proof.go`, handoff `DraftOutcome`

**Checks this session:**  
- Play Store `com.google.android.youtube` → HTTP **200**  
- `go test` focused packages (cmd + proving/microsoft + oauth/microsoft + runtime + adapters/deeplink + runtime/deeplink + stage1/openai + adapters/msteams) → **175 passed**  
- Consumers/AuthorizeChat/StartChat subset verbose → **5 passed**  
- Android `HandOffActionsTest` not re-run here (source asserts verified; evidence claims BUILD SUCCESSFUL)

---

## Standard (first principles — before hunting bugs)

### Task A — `serve-msteams-proof` wired

A strong result for **owner-operable work/school Teams Graph proof serve** must have:

**Inputs → Outputs → Algorithm**

1. **Inputs:** Owner runs a dedicated CLI; env supplies Microsoft app credentials; optional Teams tenant / redirect overrides; OpenAI key for stage1.
2. **Outputs:** A live capability flow that can authorize **Chat.ReadWrite**, smoke-list chats, and serve `msteams` — without stealing Outlook’s loopback port or mail authorize path.
3. **Algorithm (must be true):**
   1. **Separate serve surface** — `serve-msteams-proof` exists beside `serve-microsoft-proof`; wiring is real (main dispatch + live dependency), not a doc-only promise.
   2. **Port isolation** — default listen/callback port is **not** Outlook’s **9195**.
   3. **Authorize path is chat, not mail** — serve calls `AuthorizeChat` → `StartChat` with chat scopes, never mail `Start` / `Mail.*`.
   4. **Work/school tenant by default** — empty Teams tenant resolves to **`organizations`** (or an explicit work tenant), not personal MSA.
   5. **Consumers fail-closed** — if tenant/`authorize` path is personal `consumers`, authorize refuses before building a known-bad chat URL.
   6. **Outlook isolation** — personal mail proof stays on 9195 / `consumers` / `Authorize`; personal deeplink `teams` stays compose hand-off.
   7. **Evidence honesty** — live Approve / chat send may remain owner-gated; unit green ≠ Graph Approve proven.

### Task B — YouTube prepare-and-open (`Wave1Specs` 38 → 39)

A strong result for **YouTube as Wave 1 media hand-off** must have:

1. **Honest ceiling** — open/search/browse only (`hands_off`). Never claim played / watched / started playback. No pretend partner playback API.
2. **Spec contract** — id `youtube`; AppClass `media`; verbs `play` + `read`; RT-4 / Auth none / Consent A; Play package `com.google.android.youtube`; pack count **39** (append after airlines/Citymapper).
3. **Phone map** — Android Open maps YouTube display/id names to that same package.
4. **Live routing coaching** — stage1 names `media` / `youtube` / play|read / prepare-and-open and refuses played-completion language.
5. **End-to-end wiring** — Spec → media ClassMap (via Wave1Specs) → stage1 → HandOffActions → `DraftOutcome` → `serve-deeplink-proof` registers and logs youtube in the media set.
6. **Real tests** — fail if count/membership/package/verbs/ceiling, empty draft, wrong verbs, flow HandedOffTo + anti-played, or stage1 coaching break.
7. **Evidence honesty** — records Play proof, green commands, and what device path was **not** proven.

---

## Combined verdict: **Pass-with-warnings**

Both deliverables clear their product bars: Teams serve is wired on **9196** with `AuthorizeChat`, default **`organizations`**, and **consumers fail-closed**; YouTube is Spec #39 with honest hand-off wiring through companion + Android package map. Warnings are env footguns / test gaps on Teams port isolation, and Pixel Auto→Open still unproven for YouTube (plus minor comment/plan hygiene).

---

## Findings — Task A (`serve-msteams-proof`)

### Passes

- **CLI + live dependency wired.** `main.go` dispatches `serve-msteams-proof`, injects `startMSTeamsProof`, and lists the command among serve aliases (`main.go:106`, `:227-232`, `:419`). `TestMSTeamsProofServeUsesTheCapabilityFlow` exercises that branch with a fake starter (`main_test.go:444-485`).
- **Default port ≠ Outlook 9195.** Teams default redirect/listen is `127.0.0.1:9196` (`msteams_proof.go:44-52`). Outlook mail proof remains `9195` (`microsoft_proof.go:35-42`).
- **AuthorizeChat, not mail Authorize.** Serve calls `msproof.AuthorizeChat` (`msteams_proof.go:64-72`). `AuthorizeChat` starts via `Flow.StartChat` with Read+Send (`proof.go:127-132`). Outlook serve still uses mail `Authorize` / `Start`.
- **Default tenant organizations.** Empty `MICROSOFT_TEAMS_TENANT` → `TenantOrganizations` (`msteams_proof.go:35-38`; constant in `flow.go:26-28`). `TestStartChatUsesOrganizationsTenantAndChatScopes` locks `/organizations/` + `Chat.ReadWrite` and bans mail scopes (`flow_test.go:210-237`).
- **Consumers fail-closed (closes prior judge gap).** `StartChat` returns `ErrConsumersTenantForChat` when tenant is `consumers` or authorize URL contains `/consumers/` (`flow.go:189-193`). Covered by `TestStartChatRejectsConsumersTenant` and `TestAuthorizeChatRejectsConsumersTenant`. Re-run this session: those cases green.
- **Capability flow after authorize.** List-chats smoke + `NewMSTeams` (`msteams_proof.go:76-95`; runtime registers `msteams` under messaging ClassMap — `runtime/msteams.go:34-53`). Runtime compose test green (`msteams_test.go:31-58`).
- **Evidence matches the serve bar.** `wave1-msteams-work-oauth.md` records 9196, AuthorizeChat, organizations, consumers fail-closed, Approve still owner-gated — consistent with code.

### Warnings

- **Shared `MICROSOFT_REDIRECT_URI` fallback can land Teams on 9195.** If `MICROSOFT_TEAMS_REDIRECT_URI` is unset and overnight Outlook env sets `MICROSOFT_REDIRECT_URI=…:9195…`, listen address is derived from that URI (`msteams_proof.go:39-56`). Default isolation is correct; mis-set env can defeat “port ≠ 9195” in practice. Docs name the Teams-specific override, but nothing in code refuses 9195 for Teams.
- **No direct unit lock on `startMSTeamsProof` defaults.** Port/tenant/AuthorizeChat are proven by source + OAuth/proof tests; the cmd test stubs the starter and does not assert 9196/`organizations`/`AuthorizeChat` on the real function.
- **Shared `authorize` helper still defaults empty listen to 9195** (`proof.go:139-141`). Serve always passes listen address, so production path is fine; empty-config callers of `AuthorizeChat` would inherit Outlook’s port.
- **Live Approve / send still open** (acknowledged). Unit green ≠ work-tenant Chat.ReadWrite Approve proven.

Out of scope for this serve task (still true from prior judge, not re-failed here): thin Read without fetching messages; chat list pagination / empty-topic 1:1 resolve.

---

## Findings — Task B (YouTube prepare-and-open)

### Passes

- **Wave1Specs = 39; youtube is last.** Spec row: id `youtube`, package `com.google.android.youtube`, media, play|read, never-claim-played comment (`adapter.go:119-120`). Count locked via full want-table + `len(Wave1Specs())` (`adapter_test.go:67-71`); runtime count assert wants **39** (`flow_test.go:93-94`). Session count of Spec IDs: **39**, youtube index **38**.
- **Honest hands_off ceiling.** Shared deeplink `Describe()` → RT4 / HandsOff / AuthNone / ConsentA; Execute uses `handoff.DraftOutcome` (detail never claims finished/played). Empty play draft + `send` rejected (`adapter_test.go:555-567`).
- **Play package verified.** Evidence claims HTTP 200; this session re-check: **200**.
- **Stage1 coaches media play|read and refuses played claims.** Instruction line (`client.go:75`); dedicated test requires youtube/media/play/read/prepare-and-open and “never claim” + “played” (`client_test.go:545-577`).
- **Companion flow routes play+read to YouTube HandedOffTo.** `TestDeepLinkFlowRoutesMediaPlayAndReadToYouTube` bans `played`/`playing`/`started playback`/`watched` and requires “cannot know” (`flow_test.go:858-910`).
- **Android Open map matches Spec.** `"youtube"` → `com.google.android.youtube` (`HandOffActions.kt:55`); unit asserts Title Case + lowercase (`HandOffActionsTest.kt:83-84`).
- **Proof serve logs youtube in media set.** `deeplink_proof.go:37` includes `…+youtube`. Registration is dynamic from Wave1Specs (`runtime/deeplink/flow.go`).
- **Evidence honesty.** `wave1-youtube-prepare-open.md` records 38→39, Play 200, never claim played, green commands, Pixel unpaired / Auto→Open blocked. Overnight snapshot cites Wave1Specs=39.

### Warnings

- **Device Auto→Open not proven.** Pixel unpaired; package launch on device not exercised. Same standing warning as other Wave1 hand-off judges — not a Fail while evidence does not invent device success.
- **Stale human-facing header on deeplink proof.** Ready log includes youtube; package comment still speaks in Apple Music-era terms (`deeplink_proof.go:16-21`). Dynamic registration means behavior is correct; ops skim can lag.
- **Plan row hygiene (background).** Implementation plan still has mixed YouTube language elsewhere (completes vs prepare-and-open). Ship path and evidence correctly treat release as hand-off; plan drift is documentation debt, not a Spec bug.
- **Android unit test not re-run in this judge session.** Source asserts match Spec package; evidence claims BUILD SUCCESSFUL — treated as warning-level verification gap only.

---

## Gaps / risks

1. **Teams env footgun:** Prefer requiring `MICROSOFT_TEAMS_REDIRECT_URI` (or refusing redirect ports equal to Outlook 9195) so overnight Outlook env cannot collide.
2. **Optional harden:** Unit-test `startMSTeamsProof` default listen/tenant/AuthorizeChat without a full OAuth browser.
3. **Pixel stop-line:** After re-pair, Auto→preview→Open for YouTube with app installed; do not treat unit green as device proof.
4. **Ops copy:** Refresh `deeplink_proof.go` header comments; optionally align plan YouTube ceiling language with prepare-and-open.

None of these reverse the product bars for the two asked deliverables.

---

## Isolation check

| Path | Status |
|---|---|
| Outlook `serve-microsoft-proof` / 9195 / `consumers` / mail `Authorize` | **Intact** in source |
| Personal deeplink `teams` compose | **Intact** (still in Wave1Specs; separate from `msteams`) |
| Prior Wave1Specs (38) apps | **Intact** — youtube appended; count locked at 39 |
| Mail vs chat scopes | **Separated** — chat helpers do not request `Mail.*`; mail helpers unchanged |

---

## One-line next tip

Owner-Approve work Teams on **9196** with `MICROSOFT_TEAMS_REDIRECT_URI` set explicitly (avoid Outlook’s 9195 env), then re-pair Pixel for YouTube Auto→Open smoke.
