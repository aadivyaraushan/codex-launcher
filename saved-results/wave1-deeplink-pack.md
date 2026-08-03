# Wave 1 deep-link pack (Venmo / Cash App / Zelle / Starbucks / Chipotle)

**Date:** 2026-08-02  
**Purpose:** Record Group A money/food prepare-and-open adapters, the live
`serve-deeplink-proof` entrypoint, and what still needs Pixel smoke.

**Callers:** overnight Group A batch; companion `adapters/deeplink` +
`runtime/deeplink` + `serve-deeplink-proof`.  
**User ask:** keep going after Instagram Pixel proof — wire deep-link pack live.

## Result

Shared `adapters/deeplink.Wave1Specs()`:

| ID | App | Package | Verb |
|---|---|---|---|
| venmo | Venmo | `com.venmo` | compose |
| cashapp | Cash App | `com.squareup.cash` | compose |
| zelle | Zelle | `com.zellepay.zelle` | compose |
| starbucks | Starbucks | `com.starbucks.mobilecard` | order |
| chipotle | Chipotle | `com.chipotle.ordering` | order |

Ceiling `hands_off`, consent A, auth none. Outcomes use `handoff.DraftOutcome`
(no control characters; never claims paid/sent/ordered).

Android `HandOffActions` maps those display names to packages.

### Runtime + live entrypoint

`runtime/deeplink` registers all five Wave1 specs into stage2:

- `money` → venmo, cashapp, zelle (compose)
- `food` → starbucks, chipotle (order)

Stage1 OpenAI instructions list `money` as a stable app class and coach
`compose` + `money` for Venmo/Cash App/Zelle (never invent verb `pay`).
ClassMap keys are `money` + `food` only (not `payments`). Verb `pay` is
rejected at stage1 parse (`route_test.go`).

```bash
go run ./companion/cmd/codex-launcher serve-deeplink-proof
```

Needs `OPENAI_API_KEY` only. No OAuth.

## How to re-run

```bash
go test ./companion/internal/capability/adapters/deeplink/ \
        ./companion/internal/capability/runtime/deeplink/ \
        ./companion/internal/capability/handoff/

go test ./companion/cmd/codex-launcher/ -run DeepLink
```

Green this session (2026-08-02):

- `go test ./companion/internal/capability/runtime/deeplink/` → ok
- `go test ./companion/cmd/codex-launcher/ -run DeepLink` → ok
- `go test ./companion/internal/capability/adapters/deeplink/` → ok
- `go test ./companion/internal/capability/routing/stage1/openai/` → ok

## Sibling sites checked

Searched `serve-instagram-proof`, `serve-todoist-proof`, `startInstagramProof`,
`Wave1Specs`, `HandOffActions`, `com.venmo`, `com.starbucks`, `payments`,
`runtime/deeplink`.

- `main.go` wires `serve-deeplink-proof` + `needsConfiguredRuntime`
  (same shape as Instagram/Todoist).
- Android `HandOffActions` already has all five packages — no change needed.
- Stage1 rejects verb `pay`; ClassMap uses `money` + `food` only.

## Pixel smoke

### Venmo (2026-08-02) — OPEN

Device: Pixel 9 `4B230DLAQ001Z5` · `serve-deeplink-proof`  
Request id: `87d840b9-6611-44d9-b5b2-d18a3c9a6643`

1. Home Auto → Venmo draft utterance (stage1 `gpt-5.6-luna`: 306 in / 51 out)
2. Preview: `Prepare a Venmo draft` / `Venmo · compose` → **Open Venmo**
3. Execute: `reached=hands_off done=true handed_off_to=Venmo`
4. Result sheet: **Handed off**, cannot-know wording, **Copy draft**, **Open Venmo** — no paid/sent
5. Open Venmo → focus `com.venmo/...NavigationHostContainer`

### Starbucks (2026-08-02) — companion path OPEN; app launch blocked

Device: Pixel 9 `4B230DLAQ001Z5` · `serve-deeplink-proof`  
Request id: `649f7036-bd41-4492-ae00-e649b858283b`

1. Home Auto → “Order a grande oat latte at Starbucks…” (stage1: 275 in / 42 out)
2. Preview: `Prepare a Starbucks draft` / `Starbucks · order` → **Open Starbucks**
3. Execute: `reached=hands_off done=true handed_off_to=Starbucks`
4. Result sheet: **Handed off**, cannot-know wording, **Copy draft**, **Open Starbucks** — no “ordered”
5. **Open Starbucks did not leave Operator** — `com.starbucks.mobilecard` is **not installed**
   on this Pixel (`pm path` exit 1). Companion hand-off sheet is proven; package launch
   needs the Starbucks app installed (or a package-id fix if Play uses another id).

